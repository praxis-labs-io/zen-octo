// Command zen-octo is a terminal client for GitHub pull requests and issues.
package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/version"
)

func main() {
	if err := fang.Execute(context.Background(), newRootCmd(), fang.WithVersion(version.Version)); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "zen-octo",
		Short:         "A terminal client for GitHub pull requests and issues",
		Version:       version.Version,
		SilenceUsage:  true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return run() },
		SilenceErrors: false,
	}
	cmd.AddCommand(newConfigPathCmd())
	return cmd
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := gh.New()
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(app.New(cfg, client)).Run()
	return err
}

// newConfigPathCmd prints where config is read from, so "why isn't my config
// loading" is answerable without guessing.
func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config-path",
		Short: "Print the path zen-octo reads config from",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	}
}
