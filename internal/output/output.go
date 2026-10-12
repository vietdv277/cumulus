// Package output implements the global --output table|json|yaml behaviour
// shared by the command packages.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/vietdv277/cumulus/internal/config"
)

// Supported output formats.
const (
	Table = "table"
	JSON  = "json"
	YAML  = "yaml"
)

// Valid reports whether f is a supported output format.
func Valid(f string) bool {
	return f == Table || f == JSON || f == YAML
}

// Format resolves the effective output format for a running command: the
// global --output flag (when passed) > defaults.output in
// ~/.config/cml/config.yaml > table. An unknown value from either source is
// an error.
func Format(cmd *cobra.Command) (string, error) {
	if f := cmd.Root().PersistentFlags().Lookup("output"); f != nil && f.Changed {
		value := f.Value.String()
		if !Valid(value) {
			return "", InvalidError(value)
		}
		return value, nil
	}
	if saved := savedOutput(); saved != "" {
		if !Valid(saved) {
			return "", InvalidError(saved)
		}
		return saved, nil
	}
	return Table, nil
}

// InvalidError is the error for an unknown output format.
func InvalidError(value string) error {
	return fmt.Errorf("invalid output format %q (want table, json or yaml)", value)
}

// ConflictError is the error for -i/--interactive with a machine format.
func ConflictError() error {
	return fmt.Errorf("--interactive cannot be combined with --output json/yaml")
}

// savedOutput reads defaults.output from the config file; a config that
// cannot be loaded simply falls through to the table default.
func savedOutput() string {
	cfg, err := config.LoadCMLConfig()
	if err != nil || cfg.Defaults == nil {
		return ""
	}
	return cfg.Defaults.Output
}

// Render writes v to w in the given format: json (2-space indent) or yaml for
// machine output, or the command's existing table printer for table mode.
func Render(w io.Writer, format string, v any, table func()) error {
	switch format {
	case JSON:
		data, err := json.MarshalIndent(emptySliceAsArray(v), "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode json output: %w", err)
		}
		data = append(data, '\n')
		_, err = w.Write(data)
		return err
	case YAML:
		data, err := yaml.Marshal(emptySliceAsArray(v))
		if err != nil {
			return fmt.Errorf("failed to encode yaml output: %w", err)
		}
		_, err = w.Write(data)
		return err
	case Table:
		table()
		return nil
	default:
		return InvalidError(format)
	}
}

// emptySliceAsArray replaces a nil slice with an empty one so json encodes []
// instead of null.
func emptySliceAsArray(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}
