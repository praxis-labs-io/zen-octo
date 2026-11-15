// Command zen-octo is a terminal client for GitHub pull requests and issues.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
	"github.com/praxis-labs-io/zen-octo/internal/update"
	"github.com/praxis-labs-io/zen-octo/internal/version"
)

func main() {
	if err := fang.Execute(context.Background(), newRootCmd(), fang.WithVersion(version.Version)); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zen-octo [owner/repo#123 | pull request URL]",
		Short: "A terminal client for GitHub pull requests and issues",
		Long: "A terminal client for GitHub pull requests and issues.\n\n" +
			"With no argument it opens on the list. Name a pull request to open straight onto it.",
		Version:      version.Version,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mockup, err := cmd.Flags().GetBool("mockup")
			if err != nil {
				return err
			}
			return run(cmd.Context(), mockup, args)
		},
		SilenceErrors: false,
	}
	cmd.Flags().Bool("mockup", false, "Render the UI over fixture data, with no network and no account")
	cmd.AddCommand(newConfigPathCmd(), newUpdateCmd())
	return cmd
}

// client is what the UI calls, plus the lookup that resolves a pull request named on the command line.
type client interface {
	app.GitHub
	FindPullRequest(ctx context.Context, ref gh.PRRef) (gh.PullRequest, error)
}

// Bounds the lookup the same as a request from inside the UI.
const lookupTimeout = 30 * time.Second

func run(ctx context.Context, mockup bool, args []string) error {
	var ref *gh.PRRef
	if len(args) == 1 {
		parsed, err := gh.ParsePRRef(args[0])
		if err != nil {
			return err
		}
		ref = &parsed
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := newClient(mockup)
	if err != nil {
		return err
	}

	var start *gh.PullRequest
	if ref != nil {
		pr, err := findPullRequest(ctx, client, *ref)
		if err != nil {
			return err
		}
		start = &pr
	}

	surface := theme.Query(os.Stdin, os.Stdout)

	m := app.New(cfg, client, surface, releaseCheck(mockup))
	if start != nil {
		m = m.StartOn(*start)
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

func findPullRequest(ctx context.Context, c client, ref gh.PRRef) (gh.PullRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	return c.FindPullRequest(ctx, ref)
}

func releaseCheck(mockup bool) app.ReleaseCheck {
	if mockup {
		return nil
	}
	return func(ctx context.Context) (update.Result, error) {
		path, err := update.Path()
		if err != nil {
			return update.Result{}, err
		}
		return update.Check(ctx, update.Options{Current: version.Version, CachePath: path})
	}
}

func newClient(mockup bool) (client, error) {
	if mockup {
		return app.Mock{}, nil
	}

	client, err := gh.New()
	if err != nil {
		return nil, err
	}
	return client, nil
}

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
