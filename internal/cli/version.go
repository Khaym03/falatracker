package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Khaym03/falatracker/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "falatracker", version.Get())
		},
	}
}
