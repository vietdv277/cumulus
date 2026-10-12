package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/output"
)

// Output formats for the global --output flag.
const (
	OutputTable = output.Table
	OutputJSON  = output.JSON
	OutputYAML  = output.YAML
)

// outputFlag holds the global -o/--output value (registered on rootCmd in
// root.go).
var outputFlag string

// outputFormat resolves the effective output format: --output (when passed) >
// defaults.output in ~/.config/cml/config.yaml > table.
func outputFormat(c *cobra.Command) (string, error) {
	return output.Format(c)
}

// render writes v in the given format: json/yaml documents, or the existing
// table printer for table mode.
func render(w io.Writer, format string, v any, table func()) error {
	return output.Render(w, format, v, table)
}

// interactiveConflict is the error for -i/--interactive with a machine format.
func interactiveConflict() error {
	return output.ConflictError()
}
