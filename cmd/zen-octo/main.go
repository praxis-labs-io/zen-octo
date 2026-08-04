// Command zen-octo is a terminal client for GitHub pull requests and issues.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/zen-octo/zen-octo/internal/version"
)

func main() {
	if err := fang.Execute(context.Background(), newRootCmd(), fang.WithVersion(version.Version)); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "zen-octo",
		Short:   "A terminal client for GitHub pull requests and issues",
		Version: version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "zen-octo", version.Version)
			return err
		},
	}
}
