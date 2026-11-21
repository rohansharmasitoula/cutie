// Command cutie is a terminal dashboard for your GitHub pull requests and the coding
// agents working on them in herdr.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/rohansharmasitoula/cutie/internal/agents"
	"github.com/rohansharmasitoula/cutie/internal/config"
	"github.com/rohansharmasitoula/cutie/internal/state"
	"github.com/rohansharmasitoula/cutie/internal/ui"
	"github.com/rohansharmasitoula/cutie/internal/utils"
)

// version is set by release builds (.goreleaser.yaml); go install builds fall back to
// the module version Go records in the binary.
var version = ""

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("cutie", buildVersion())
		return
	}
	// The scripted agent CUTIE_DEMO=herdr starts in its panes; not for direct use.
	if len(os.Args) > 2 && os.Args[1] == "demo-agent" {
		promptFile := ""
		if len(os.Args) > 3 {
			promptFile = os.Args[3]
		}
		if err := ui.RunDemoAgent(os.Args[2], promptFile); err != nil {
			fmt.Fprintln(os.Stderr, "cutie demo-agent:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cutie:", err)
		os.Exit(1)
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.Main.Version
	}
	return "unknown"
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Version, cfg.ReleaseBuild = buildVersion(), version != ""
	promptDir := filepath.Join(state.Dir(), "prompts")
	if cfg.Demo { // demo agents show the built-in prompts, not your own
		promptDir = ""
	}
	prompts, err := agents.LoadPrompts(promptDir)
	if err != nil {
		return utils.Wrap("loading prompts", err)
	}
	watches, err := state.LoadWatches()
	if err != nil {
		return utils.Wrap("loading watches", err)
	}
	queue, err := state.LoadQueue()
	if err != nil {
		return utils.Wrap("loading the review queue", err)
	}
	modes, err := state.LoadModes()
	if err != nil {
		return utils.Wrap("loading review modes", err)
	}
	sizes, err := state.LoadSizes()
	if err != nil {
		return utils.Wrap("loading sizes", err)
	}
	return ui.Run(cfg, prompts, watches, queue, modes, sizes)
}
