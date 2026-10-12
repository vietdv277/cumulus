package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// versionOutput is the stable json/yaml shape of `cml version`.
type versionOutput struct {
	Version   string `json:"version" yaml:"version"`
	Commit    string `json:"commit" yaml:"commit"`
	BuildDate string `json:"build_date" yaml:"build_date"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := outputFormat(cmd)
		if err != nil {
			return err
		}

		info := versionOutput{Version: Version, Commit: Commit, BuildDate: BuildDate}
		return render(os.Stdout, out, info, func() {
			fmt.Printf("Cumulus CLI\n")
			fmt.Printf("  Version:    %s\n", Version)
			fmt.Printf("  Commit:     %s\n", Commit)
			fmt.Printf("  Build Date: %s\n", BuildDate)
		})
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
