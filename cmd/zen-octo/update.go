package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/praxis-labs-io/zen-octo/internal/update"
	"github.com/praxis-labs-io/zen-octo/internal/version"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Install the latest release over this binary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := update.InstallDir()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()

			result, err := update.Check(ctx, update.Options{Current: version.Version})
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Could not check for a newer release: %v\n", err)
			}

			switch {
			case version.Version == update.DevVersion:
				_, _ = fmt.Fprintln(out, "This is a locally built binary. Installing the latest release over it.")
			case result.Available:
				_, _ = fmt.Fprintf(out, "%s is available, running v%s.\n", result.Latest, strings.TrimPrefix(version.Version, "v"))
			case result.Latest != "":
				_, _ = fmt.Fprintf(out, "%s is the latest release. Nothing to install.\n", result.Latest)
				return nil
			}

			if err := update.Install(ctx, update.InstallOptions{Dir: dir, Out: out}); err != nil {
				return fmt.Errorf("updating: %w", err)
			}
			return nil
		},
	}
}
