package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var logsCmd = &cobra.Command{
	Use:     "logs <target>",
	Aliases: []string{"log"},
	Short:   "Query and tail logs",
	Long: `Query or tail logs in the current context.

On AWS a target is a CloudWatch log group and --filter is a CloudWatch
filter pattern. On GCP a target is a Cloud Logging log name (e.g.
cloudaudit.googleapis.com/activity) and --filter is a Logging query
expression. 'logs list' shows the available targets.

Examples:
  cml logs /aws/lambda/api            # last 15 minutes
  cml logs /aws/lambda/api --since 1h --limit 50
  cml logs /aws/lambda/api --filter ERROR
  cml logs /aws/lambda/api -f         # tail (Ctrl-C to stop)
  cml logs list /aws/lambda/          # list log groups by prefix
  cml logs run.googleapis.com/stderr --filter 'severity>=ERROR'   # GCP`,
	Args: cobra.ExactArgs(1),
	RunE: runLogsQuery,
}

var logsListCmd = &cobra.Command{
	Use:   "list [prefix]",
	Short: "List log groups (AWS) or log names (GCP)",
	Long: `List log groups (AWS) or log names (GCP) in the current context,
optionally filtered by name prefix.

Examples:
  cml logs list                # all log groups
  cml logs list /aws/lambda/   # groups under a prefix`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogsList,
}

var (
	logsContextFlag string
	logsFollow      bool
	logsSince       string
	logsFilter      string
	logsLimit       int
)

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsListCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Tail the target continuously (Ctrl-C to stop)")
	logsCmd.Flags().StringVar(&logsSince, "since", "15m", "How far back to start (Go duration, e.g. 15m, 1h)")
	logsCmd.Flags().StringVar(&logsFilter, "filter", "", "Filter: CloudWatch pattern (AWS) or Logging query (GCP)")
	logsCmd.Flags().IntVar(&logsLimit, "limit", 100, "Max entries returned (query only)")

	logsCmd.PersistentFlags().StringVarP(&logsContextFlag, "context", "c", "", "Use specific context")
}

func runLogsQuery(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	if logsFollow {
		if out == OutputYAML {
			return fmt.Errorf("use -o json with --follow")
		}
		if f := cmd.Flags().Lookup("limit"); f != nil && f.Changed {
			return fmt.Errorf("--limit applies to query only")
		}
	}

	since, err := parseLogsSince(logsSince)
	if err != nil {
		return err
	}
	opts := &provider.LogsOptions{Since: since, Filter: logsFilter, Limit: logsLimit}

	if logsFollow {
		return runLogsFollow(out, args[0], opts)
	}

	entries, err := logsQuery(args[0], opts)
	if err != nil {
		return err
	}

	if len(entries) == 0 && out == OutputTable {
		fmt.Println("No log entries found")
		return nil
	}
	return render(os.Stdout, out, entries, func() { printLogEntries(entries) })
}

// runLogsFollow tails a log group, printing each entry as it arrives. A
// Ctrl-C cancels the context; Tail then stops cleanly and the command
// exits 0.
func runLogsFollow(out, target string, opts *provider.LogsOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logsProvider, err := getLogsProvider(ctx)
	if err != nil {
		return err
	}

	return logsProvider.Tail(ctx, target, opts, func(e types.LogEntry) error {
		if out == OutputJSON {
			line, err := formatLogEntryJSON(e)
			if err != nil {
				return err
			}
			fmt.Println(line)
			return nil
		}
		fmt.Println(formatLogEntry(e))
		return nil
	})
}

func runLogsList(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	prefix := ""
	if len(args) > 0 {
		prefix = args[0]
	}

	logsProvider, err := getLogsProvider(context.Background())
	if err != nil {
		return err
	}

	targets, err := logsProvider.ListTargets(context.Background(), prefix)
	if err != nil {
		return err
	}

	if len(targets) == 0 && out == OutputTable {
		fmt.Println("No log groups found")
		return nil
	}
	return render(os.Stdout, out, targets, func() { printLogTargetsTable(targets) })
}

// getLogsProvider resolves the context and returns its logs provider.
func getLogsProvider(ctx context.Context) (provider.LogsProvider, error) {
	ctxConfig, ctxName, err := resolveContext(logsContextFlag)
	if err != nil {
		return nil, err
	}

	switch ctxConfig.Provider {
	case "aws":
		client, err := aws.NewClient(ctx,
			aws.WithProfile(ctxConfig.Profile),
			aws.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create AWS client: %w", err)
		}
		return aws.NewLogsProvider(client), nil

	case "gcp":
		return newGCPLogsProvider(ctx, ctxConfig)

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

// newGCPLogsProvider builds the Cloud Logging provider; a variable so tests can
// check the routing without GCP credentials.
var newGCPLogsProvider = func(ctx context.Context, ctxConfig *config.Context) (provider.LogsProvider, error) {
	client, err := gcp.NewClient(ctx,
		gcp.WithProject(ctxConfig.Project),
		gcp.WithRegion(ctxConfig.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP client: %w", err)
	}
	return gcp.NewLogsProvider(client), nil
}

// logsQuery runs a one-shot query against the target.
func logsQuery(target string, opts *provider.LogsOptions) ([]types.LogEntry, error) {
	logsProvider, err := getLogsProvider(context.Background())
	if err != nil {
		return nil, err
	}
	return logsProvider.Query(context.Background(), target, opts)
}

// parseLogsSince parses the --since flag value as a positive Go duration.
func parseLogsSince(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --since %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid --since %q: must be positive", s)
	}
	return d, nil
}

// formatLogEntry renders one entry as a text line:
// 2026-10-12T09:15:02Z [stream] LEVEL message (level omitted when empty).
func formatLogEntry(e types.LogEntry) string {
	ts := e.Timestamp.UTC().Format(time.RFC3339)
	if e.Level == "" {
		return fmt.Sprintf("%s [%s] %s", ts, e.Source, e.Message)
	}
	return fmt.Sprintf("%s [%s] %s %s", ts, e.Source, e.Level, e.Message)
}

// formatLogEntryJSON renders one entry as a single-line JSON object, for
// NDJSON output while following.
func formatLogEntryJSON(e types.LogEntry) (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("failed to encode log entry: %w", err)
	}
	return string(data), nil
}

// printLogEntries prints one text line per entry.
func printLogEntries(entries []types.LogEntry) {
	for _, e := range entries {
		fmt.Println(formatLogEntry(e))
	}
}

// printLogTargetsTable prints log groups in a table format.
func printLogTargetsTable(targets []types.LogTarget) {
	headers := []string{"NAME", "CREATED", "SIZE"}
	widths := []int{40, 18, 12}

	var sb strings.Builder

	// Top border
	sb.WriteString(ui.BorderStyle.Render(ui.TopLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.TopT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	for i, h := range headers {
		cell := " " + padRightLogTargets(h, widths[i]) + " "
		sb.WriteString(ui.HeaderStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(ui.BorderStyle.Render(ui.LeftT))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.Cross))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.RightT))
	sb.WriteString("\n")

	// Data rows
	for _, t := range targets {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell := " " + padRightLogTargets(t.Name, widths[0]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		created := "-"
		if !t.CreatedAt.IsZero() {
			created = t.CreatedAt.Format("2006-01-02 15:04")
		}
		cell = " " + padRightLogTargets(created, widths[1]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		size := "-"
		if t.SizeBytes != 0 {
			size = formatLogBytes(t.SizeBytes)
		}
		cell = " " + padRightLogTargets(size, widths[2]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(ui.BorderStyle.Render(ui.BottomLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.BottomT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d log groups\n", len(targets))
}

// formatLogBytes renders a byte count with binary units.
func formatLogBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	i := -1
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func padRightLogTargets(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
