package main

import (
	"assistente/internal/installpath"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:         "_installer-path [add|remove]",
		Hidden:      true,
		Annotations: map[string]string{"skipInit": "true"},
		Args:        cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:   []string{"add", "remove"},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := installpath.Run(args[0] == "remove")
			var warning *installpath.CleanupWarning
			if errors.As(err, &warning) {
				_, printErr := fmt.Fprintln(cmd.OutOrStdout(), warning)
				return printErr
			}
			return err
		},
	})
}
