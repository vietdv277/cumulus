package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vietdv277/cumulus/pkg/types"
)

// setOutputFlagState sets the persistent --output flag's value and Changed
// state so one test's cobra parse cannot leak into another.
func setOutputFlagState(t *testing.T, value string) {
	t.Helper()
	f := rootCmd.PersistentFlags().Lookup("output")
	if f == nil {
		t.Fatal("--output flag not registered on rootCmd")
	}
	if err := f.Value.Set(value); err != nil {
		t.Fatalf("set --output: %v", err)
	}
	f.Changed = value != ""
	t.Cleanup(func() {
		_ = f.Value.Set("")
		f.Changed = false
	})
}

func TestOutputFormatPrecedence(t *testing.T) {
	baseConfig := `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
`

	tests := []struct {
		name      string
		flagValue string // "" = flag not passed
		config    string
		want      string
		wantErr   string
	}{
		{
			name:      "flag overrides config",
			flagValue: "json",
			config:    baseConfig + "defaults:\n  output: yaml\n",
			want:      "json",
		},
		{
			name:   "config used without flag",
			config: baseConfig + "defaults:\n  output: yaml\n",
			want:   "yaml",
		},
		{
			name:   "neither flag nor config",
			config: baseConfig,
			want:   "table",
		},
		{
			name:      "invalid flag value",
			flagValue: "xml",
			config:    baseConfig,
			wantErr:   `invalid output format "xml" (want table, json or yaml)`,
		},
		{
			name:    "invalid config value",
			config:  baseConfig + "defaults:\n  output: xml\n",
			wantErr: `invalid output format "xml" (want table, json or yaml)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestConfig(t, tt.config)
			setOutputFlagState(t, tt.flagValue)

			got, err := outputFormat(rootCmd)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("format = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderGolden(t *testing.T) {
	vms := []types.VM{{
		ID:         "i-0abc123def456",
		Name:       "web-01",
		State:      types.VMStateRunning,
		PrivateIP:  "10.0.0.1",
		PublicIP:   "1.2.3.4",
		Type:       "t3.micro",
		Zone:       "us-east-1a",
		Tags:       map[string]string{"env": "prod"},
		LaunchedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Provider:   "aws",
	}}

	t.Run("json exact", func(t *testing.T) {
		var buf bytes.Buffer
		if err := render(&buf, OutputJSON, vms, nil); err != nil {
			t.Fatalf("render: %v", err)
		}
		want := `[
  {
    "id": "i-0abc123def456",
    "name": "web-01",
    "state": "running",
    "private_ip": "10.0.0.1",
    "public_ip": "1.2.3.4",
    "type": "t3.micro",
    "zone": "us-east-1a",
    "tags": {
      "env": "prod"
    },
    "launched_at": "2026-01-02T03:04:05Z",
    "provider": "aws"
  }
]
`
		if buf.String() != want {
			t.Fatalf("json mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
		}
	})

	t.Run("yaml exact", func(t *testing.T) {
		var buf bytes.Buffer
		if err := render(&buf, OutputYAML, vms, nil); err != nil {
			t.Fatalf("render: %v", err)
		}
		want := `- id: i-0abc123def456
  name: web-01
  state: running
  private_ip: 10.0.0.1
  public_ip: 1.2.3.4
  type: t3.micro
  zone: us-east-1a
  tags:
    env: prod
  launched_at: 2026-01-02T03:04:05Z
  provider: aws
`
		if buf.String() != want {
			t.Fatalf("yaml mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
		}
	})

	t.Run("json empty slice", func(t *testing.T) {
		var buf bytes.Buffer
		if err := render(&buf, OutputJSON, []types.VM{}, nil); err != nil {
			t.Fatalf("render: %v", err)
		}
		if buf.String() != "[]\n" {
			t.Fatalf("got %q, want %q", buf.String(), "[]\n")
		}
	})

	t.Run("yaml empty slice", func(t *testing.T) {
		var buf bytes.Buffer
		if err := render(&buf, OutputYAML, []types.VM{}, nil); err != nil {
			t.Fatalf("render: %v", err)
		}
		if buf.String() != "[]\n" {
			t.Fatalf("got %q, want %q", buf.String(), "[]\n")
		}
	})

	t.Run("json nil slice", func(t *testing.T) {
		var buf bytes.Buffer
		var nilVMs []types.VM
		if err := render(&buf, OutputJSON, nilVMs, nil); err != nil {
			t.Fatalf("render: %v", err)
		}
		if buf.String() != "[]\n" {
			t.Fatalf("got %q, want %q", buf.String(), "[]\n")
		}
	})

	t.Run("table mode calls the printer", func(t *testing.T) {
		var buf bytes.Buffer
		called := false
		if err := render(&buf, OutputTable, vms, func() { called = true }); err != nil {
			t.Fatalf("render: %v", err)
		}
		if !called {
			t.Fatal("table printer not called")
		}
		if buf.Len() != 0 {
			t.Fatalf("table mode wrote %q, want empty", buf.String())
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		var buf bytes.Buffer
		err := render(&buf, "xml", vms, nil)
		if err == nil || err.Error() != `invalid output format "xml" (want table, json or yaml)` {
			t.Fatalf("err = %v, want invalid output format error", err)
		}
	})
}

// TestInteractiveWithJSONOutput runs `vm list -i -o json` through rootCmd
// against a temp config; the conflict must fail before any cloud call.
func TestInteractiveWithJSONOutput(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	if f := vmListCmd.Flags().Lookup("interactive"); f != nil {
		t.Cleanup(func() {
			_ = f.Value.Set("false")
			f.Changed = false
		})
	}
	prevSilence := rootCmd.SilenceErrors
	rootCmd.SilenceErrors = true
	t.Cleanup(func() { rootCmd.SilenceErrors = prevSilence })

	rootCmd.SetArgs([]string{"vm", "list", "-i", "-o", "json"})
	t.Cleanup(func() { rootCmd.SetArgs([]string{}) })

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for -i with -o json, got nil")
	}
	if !strings.Contains(err.Error(), "--interactive cannot be combined with --output json/yaml") {
		t.Fatalf("error = %v, want interactive/output conflict", err)
	}
}

// TestVersionJSONOutput runs `version -o json` through rootCmd and asserts the
// whole stdout is one JSON document.
func TestVersionJSONOutput(t *testing.T) {
	setTestConfig(t, testConfigYAML)
	setOutputFlagState(t, "") // reset any leaked state; args provide -o json

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	rootCmd.SetArgs([]string{"version", "-o", "json"})
	t.Cleanup(func() { rootCmd.SetArgs([]string{}) })
	execErr := rootCmd.Execute()

	w.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}

	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	got := string(data)
	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\ngot: %q", err, got)
	}
	for _, key := range []string{"version", "commit", "build_date"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("missing key %q in %v", key, doc)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("json output missing trailing newline: %q", got)
	}
}

// TestContextsJSONStdoutPurity runs `contexts -o json` through rootCmd against
// a temp config; stdout must be exactly the JSON document — progress, hints
// and table headers go to stderr.
func TestContextsJSONStdoutPurity(t *testing.T) {
	setTestConfig(t, testConfigYAML)
	setOutputFlagState(t, "") // args provide -o json

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	rootCmd.SetArgs([]string{"contexts", "-o", "json"})
	t.Cleanup(func() { rootCmd.SetArgs([]string{}) })
	execErr := rootCmd.Execute()

	w.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}

	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	got := string(data)
	var doc []map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("stdout is not a pure JSON document: %v\ngot: %q", err, got)
	}
	if len(doc) != 2 || doc[0]["name"] != "aws:prod" || doc[0]["current"] != true {
		t.Fatalf("unexpected document: %v", doc)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("json output missing trailing newline: %q", got)
	}
}
