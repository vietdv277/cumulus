package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

func TestParseLogsSince(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"15m", 15 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"2s", 2 * time.Second, false},
		{"15x", 0, true},
		{"", 0, true},
		{"0s", 0, true},
		{"-5m", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseLogsSince(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseLogsSince(%q) error = nil, want error", tt.in)
				}
				if !strings.Contains(err.Error(), "invalid --since") {
					t.Errorf("parseLogsSince(%q) error = %v, want --since mention", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLogsSince(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseLogsSince(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatLogEntry(t *testing.T) {
	ts := time.Date(2026, 10, 12, 9, 15, 2, 0, time.UTC)
	tests := []struct {
		name  string
		entry types.LogEntry
		want  string
	}{
		{
			name:  "with level",
			entry: types.LogEntry{Timestamp: ts, Source: "stream", Level: "ERROR", Message: "boom"},
			want:  "2026-10-12T09:15:02Z [stream] ERROR boom",
		},
		{
			name:  "empty level omitted",
			entry: types.LogEntry{Timestamp: ts, Source: "stream", Message: "hello"},
			want:  "2026-10-12T09:15:02Z [stream] hello",
		},
		{
			name:  "non-UTC timestamp converted",
			entry: types.LogEntry{Timestamp: ts.In(time.FixedZone("W5", -5*3600)), Source: "s", Message: "m"},
			want:  "2026-10-12T09:15:02Z [s] m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLogEntry(tt.entry); got != tt.want {
				t.Errorf("formatLogEntry() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLogEntryJSON(t *testing.T) {
	entry := types.LogEntry{
		Timestamp: time.Date(2026, 10, 12, 9, 15, 2, 0, time.UTC),
		Source:    "stream",
		Level:     "ERROR",
		Message:   "boom",
		Provider:  "aws",
	}

	line, err := formatLogEntryJSON(entry)
	if err != nil {
		t.Fatalf("formatLogEntryJSON() unexpected error: %v", err)
	}
	if !json.Valid([]byte(line)) {
		t.Fatalf("formatLogEntryJSON() = %q, want valid single-line json", line)
	}
	var got types.LogEntry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != entry {
		t.Errorf("round-trip = %+v, want %+v", got, entry)
	}
}

func TestFormatLogBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
		{5 * 1024 * 1024 * 1024, "5.0 GiB"},
	}

	for _, tt := range tests {
		if got := formatLogBytes(tt.in); got != tt.want {
			t.Errorf("formatLogBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// runLogsCmd executes the logs command against the test config, silencing
// cobra's own printing, and resets the logs flag vars afterwards so one
// test's parse cannot leak into another.
func runLogsCmd(t *testing.T, args ...string) error {
	t.Helper()
	setTestConfig(t, testConfigYAML)

	rootCmd.SilenceUsage = true
	t.Cleanup(func() {
		rootCmd.SilenceUsage = false
		resetLegacyFlags()
		resetLogsFlags()
	})

	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// resetLogsFlags clears flag values and Changed state left over from a cobra
// parse; flag values persist between parses on a shared FlagSet.
func resetLogsFlags() {
	logsFollow = false
	logsSince = "15m"
	logsFilter = ""
	logsLimit = 100
	for _, name := range []string{"follow", "since", "filter", "limit"} {
		if f := logsCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
	if f := rootCmd.PersistentFlags().Lookup("output"); f != nil {
		f.Changed = false
	}
}

func TestLogsFollowRejectsYAMLOutput(t *testing.T) {
	err := runLogsCmd(t, "logs", "mygroup", "-f", "-o", "yaml")
	if err == nil {
		t.Fatal("expected error for --follow with -o yaml")
	}
	if err.Error() != "use -o json with --follow" {
		t.Errorf("err = %q, want %q", err.Error(), "use -o json with --follow")
	}
}

func TestLogsFollowRejectsLimitFlag(t *testing.T) {
	err := runLogsCmd(t, "logs", "mygroup", "-f", "--limit", "5")
	if err == nil {
		t.Fatal("expected error for --follow with --limit")
	}
	if err.Error() != "--limit applies to query only" {
		t.Errorf("err = %q, want %q", err.Error(), "--limit applies to query only")
	}
}

func TestLogsRejectsBadSince(t *testing.T) {
	err := runLogsCmd(t, "logs", "mygroup", "--since", "15x")
	if err == nil {
		t.Fatal("expected error for a bad --since value")
	}
	if !strings.Contains(err.Error(), `invalid --since "15x"`) {
		t.Errorf("err = %q, want invalid --since mention", err.Error())
	}
}

func TestLogsRejectsProfileFlag(t *testing.T) {
	err := runLogsCmd(t, "logs", "list", "-p", "dev")
	if err == nil {
		t.Fatal("expected error when -p is passed to cml logs")
	}
	want := "--profile is not supported by context-aware commands; use --context <name> or 'cml use <context>'"
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

func TestLogsGCPContextRoutesToCloudLogging(t *testing.T) {
	orig := newGCPLogsProvider
	t.Cleanup(func() { newGCPLogsProvider = orig })
	errStub := errors.New("gcp logs provider called")

	// Direct context name and its alias must both reach the GCP provider with
	// the resolved context.
	for _, flag := range []string{"gcp:staging", "stg"} {
		var gotProject string
		newGCPLogsProvider = func(_ context.Context, c *config.Context) (provider.LogsProvider, error) {
			gotProject = c.Project
			return nil, errStub
		}
		err := runLogsCmd(t, "logs", "mygroup", "-c", flag)
		if !errors.Is(err, errStub) {
			t.Fatalf("-c %s: err = %v, want the GCP provider to be used", flag, err)
		}
		if gotProject == "" {
			t.Errorf("-c %s: GCP provider got an empty project", flag)
		}
	}
}
