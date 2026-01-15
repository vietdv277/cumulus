package cmd

import (
    "fmt"

    "github.com/spf13/cobra"
)

var (
    Version   = "dev"
    Commit    = "none"
    BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("Cumulus CLI\n")
        fmt.Printf("  Version:    %s\n", Version)
        fmt.Printf("  Commit:     %s\n", Commit)
        fmt.Printf("  Build Date: %s\n", BuildDate)
    },
}

func init() {
    rootCmd.AddCommand(versionCmd)
}
