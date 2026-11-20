// Command cutie is a terminal dashboard for your GitHub pull requests and the coding
// agents working on them in herdr.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rohansharmasitoula/cutie/internal/agents"
	"github.com/rohansharmasitoula/cutie/internal/config"
	"github.com/rohansharmasitoula/cutie/internal/state"
	"github.com/rohansharmasitoula/cutie/internal/ui"
	"github.com/rohansharmasitoula/cutie/internal/utils"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cutie:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	prompts, err := agents.LoadPrompts(filepath.Join(state.Dir(), "prompts"))
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
	sizes, err := state.LoadSizes()
	if err != nil {
		return utils.Wrap("loading sizes", err)
	}
	return ui.Run(cfg, prompts, watches, queue, sizes)
}
