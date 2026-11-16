package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Limit is one Claude plan window (5 hour or 7 day) from `omp usage`.
type Limit struct {
	Window   string // "5h", "7d"
	Used     float64
	ResetsAt time.Time
}

// FetchLimits reads the Anthropic plan windows that every Claude model shares.
func FetchLimits() ([]Limit, error) {
	out, err := exec.Command("omp", "usage", "--json", "-p", "anthropic").Output()
	if err != nil {
		return nil, fmt.Errorf("omp usage: %w", err)
	}
	var resp struct {
		Reports []struct {
			Limits []struct {
				ID     string `json:"id"`
				Window struct {
					ID       string `json:"id"`
					ResetsAt int64  `json:"resetsAt"`
				} `json:"window"`
				Amount struct {
					UsedFraction float64 `json:"usedFraction"`
				} `json:"amount"`
			} `json:"limits"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("omp usage: decoding: %w", err)
	}
	var limits []Limit
	for _, r := range resp.Reports {
		for _, l := range r.Limits {
			if l.ID != "anthropic:"+l.Window.ID { // skip per-model windows like anthropic:7d:fable
				continue
			}
			limits = append(limits, Limit{Window: l.Window.ID, Used: l.Amount.UsedFraction, ResetsAt: time.UnixMilli(l.Window.ResetsAt)})
		}
	}
	return limits, nil
}

type Share struct {
	Name string
	Cost float64
}

// Stats is the last 24 hours of model usage from `omp stats`.
type Stats struct {
	Cost         float64
	Tokens       int64
	HourlyTokens []int64 // oldest first
	Models       []Share // by cost, highest first
	Folders      []Share // by cost, highest first
}

// FetchStats runs `omp stats --json`, which first syncs new session files (a few seconds).
func FetchStats(root string) (Stats, error) {
	out, err := exec.Command("omp", "stats", "--json").Output()
	if err != nil {
		return Stats{}, fmt.Errorf("omp stats: %w", err)
	}
	var resp struct {
		Overall struct {
			TotalCost float64 `json:"totalCost"`
		} `json:"overall"`
		TimeSeries []struct {
			Timestamp int64   `json:"timestamp"`
			Tokens    int64   `json:"tokens"`
			Cost      float64 `json:"cost"`
		} `json:"timeSeries"`
		ByModel []struct {
			Model     string  `json:"model"`
			TotalCost float64 `json:"totalCost"`
		} `json:"byModel"`
		ByFolder []struct {
			Folder    string  `json:"folder"`
			TotalCost float64 `json:"totalCost"`
		} `json:"byFolder"`
	}
	// The JSON follows "Syncing…" progress lines on stdout.
	if i := bytes.IndexByte(out, '{'); i > 0 {
		out = out[i:]
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return Stats{}, fmt.Errorf("omp stats: decoding: %w", err)
	}
	s := Stats{Cost: resp.Overall.TotalCost}
	sort.Slice(resp.TimeSeries, func(i, j int) bool { return resp.TimeSeries[i].Timestamp < resp.TimeSeries[j].Timestamp })
	for _, t := range resp.TimeSeries {
		s.HourlyTokens = append(s.HourlyTokens, t.Tokens)
		s.Tokens += t.Tokens
	}
	for _, m := range resp.ByModel {
		s.Models = append(s.Models, Share{strings.TrimPrefix(m.Model, "claude-"), m.TotalCost})
	}
	prefix := sessionFolderPrefix(root)
	for _, f := range resp.ByFolder {
		name := strings.TrimPrefix(f.Folder, prefix)
		if name == "" {
			name = filepath.Base(root)
		}
		s.Folders = append(s.Folders, Share{strings.Trim(name, "-"), f.TotalCost})
	}
	byCost := func(xs []Share) { sort.Slice(xs, func(i, j int) bool { return xs[i].Cost > xs[j].Cost }) }
	byCost(s.Models)
	byCost(s.Folders)
	return s, nil
}

// sessionFolderPrefix is how omp names the session folder of root: the path below
// $HOME with "/" turned into "-", e.g. ~/Development/work → "-Development-work".
func sessionFolderPrefix(root string) string {
	home, _ := os.UserHomeDir()
	rel, err := filepath.Rel(home, root)
	if err != nil {
		rel = root
	}
	return "-" + strings.ReplaceAll(rel, string(filepath.Separator), "-")
}

// CostMeter sums the cost of omp sessions, including every nested subagent session,
// reading each file only from where it stopped last time.
type CostMeter struct {
	files map[string]*fileCost
}

type fileCost struct {
	offset int64
	cost   float64
}

func NewCostMeter() *CostMeter { return &CostMeter{files: map[string]*fileCost{}} }

// SessionCost is the total cost of the session at path (…/x.jsonl) and of the subagent
// sessions omp stores under the sibling folder …/x/.
func (c *CostMeter) SessionCost(path string) (float64, error) {
	paths := []string{path}
	dir := strings.TrimSuffix(path, ".jsonl")
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, p := range paths {
		v, err := c.fileTotal(p)
		if err != nil {
			return 0, err
		}
		total += v
	}
	return total, nil
}

type usageLine struct {
	Type    string `json:"type"`
	Usage   *usage `json:"usage"`
	Message *struct {
		Role  string `json:"role"`
		Usage *usage `json:"usage"`
	} `json:"message"`
}

type usage struct {
	Cost struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

func (c *CostMeter) fileTotal(path string) (float64, error) {
	fc := c.files[path]
	if fc == nil {
		fc = &fileCost{}
		c.files[path] = fc
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(fc.offset, io.SeekStart); err != nil {
		return 0, err
	}
	r := bufio.NewReaderSize(f, 256*1024)
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			break // a partial last line is read again next time, once complete
		}
		if err != nil {
			return 0, err
		}
		fc.offset += int64(len(line))
		// Cheap filter: only lines that carry usage are decoded.
		if !bytes.Contains(line, []byte(`"usage"`)) {
			continue
		}
		var ul usageLine
		if json.Unmarshal(line, &ul) != nil {
			continue
		}
		switch {
		case ul.Type == "model_usage" && ul.Usage != nil:
			fc.cost += ul.Usage.Cost.Total
		case ul.Type == "message" && ul.Message != nil && ul.Message.Role == "assistant" && ul.Message.Usage != nil:
			fc.cost += ul.Message.Usage.Cost.Total
		}
	}
	return fc.cost, nil
}
