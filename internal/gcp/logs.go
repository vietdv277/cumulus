package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/option"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

const (
	defaultLogLimit  = 100
	defaultPollEvery = 2 * time.Second
	providerGCPLogs  = "gcp"
)

// errPagesDone stops pagination once the query limit is reached.
var errPagesDone = errors.New("pages done")

// GCPLogsProvider implements provider.LogsProvider for Cloud Logging.
type GCPLogsProvider struct {
	client *Client
	svc    *logging.Service

	// now and sleep are injectable so Tail tests run fast and deterministically.
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

var _ provider.LogsProvider = (*GCPLogsProvider)(nil)

// NewLogsProvider creates a new Cloud Logging provider backed by the given
// Client.
func NewLogsProvider(c *Client) *GCPLogsProvider {
	return &GCPLogsProvider{
		client: c,
		now:    time.Now,
		sleep:  sleepCtx,
	}
}

// newService builds the Cloud Logging REST client from the GCP client's ADC
// credentials; an existing client is reused so tests can inject a fake.
func (p *GCPLogsProvider) newService(ctx context.Context) (*logging.Service, error) {
	if p.svc != nil {
		return p.svc, nil
	}
	svc, err := logging.NewService(ctx, option.WithTokenSource(p.client.Credentials().TokenSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Logging client: %w", err)
	}
	p.svc = svc
	return p.svc, nil
}

// ListTargets returns logs in the context project whose short name starts
// with prefix.
func (p *GCPLogsProvider) ListTargets(ctx context.Context, prefix string) ([]types.LogTarget, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}

	var targets []types.LogTarget
	err = svc.Projects.Logs.List("projects/"+p.client.Project()).Pages(ctx, func(page *logging.ListLogsResponse) error {
		for _, name := range page.LogNames {
			short := logShortName(name)
			if prefix != "" && !strings.HasPrefix(short, prefix) {
				continue
			}
			targets = append(targets, types.LogTarget{Name: short, Provider: providerGCPLogs})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list logs: %w", err)
	}
	return targets, nil
}

// Query returns entries in target newer than now-opts.Since, oldest first,
// at most opts.Limit (default 100).
func (p *GCPLogsProvider) Query(ctx context.Context, target string, opts *provider.LogsOptions) ([]types.LogEntry, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}
	limit, since, extra := resolveLogOpts(p.now, opts)
	return p.readEntries(ctx, svc, target, since, extra, limit, map[string]bool{})
}

// Tail emits the same initial window as Query, then polls for newer entries
// every opts.PollInterval (default 2s) until ctx is cancelled or emit fails.
// Entries are never emitted twice: dedupe is by insertId, which also keeps
// later entries that share the newest timestamp.
func (p *GCPLogsProvider) Tail(ctx context.Context, target string, opts *provider.LogsOptions, emit func(types.LogEntry) error) error {
	svc, err := p.newService(ctx)
	if err != nil {
		return err
	}
	limit, since, extra := resolveLogOpts(p.now, opts)
	interval := defaultPollEvery
	if opts != nil && opts.PollInterval > 0 {
		interval = opts.PollInterval
	}

	seen := map[string]bool{}
	entries, err := p.readEntries(ctx, svc, target, since, extra, limit, seen)
	if err != nil {
		return err
	}
	newest := newestTimestamp(entries)
	for _, e := range entries {
		if err := emit(e); err != nil {
			return err
		}
	}

	for {
		// Cancellation ends the tail cleanly; any other sleep failure stops it too.
		if err := p.sleep(ctx, interval); err != nil {
			return nil
		}
		// Re-query from the newest timestamp so entries sharing it but with a
		// new insertId are still picked up; seen filters out the duplicates.
		start := newest
		if start.IsZero() {
			start = since
		}
		fresh, err := p.readEntries(ctx, svc, target, start, extra, 0, seen)
		if err != nil {
			return err
		}
		for _, e := range fresh {
			if err := emit(e); err != nil {
				return err
			}
			if e.Timestamp.After(newest) {
				newest = e.Timestamp
			}
		}
	}
}

// readEntries pages through entries matching target from start on, skipping
// insertIds already in seen, and stops at limit entries (<=0 means unlimited).
func (p *GCPLogsProvider) readEntries(ctx context.Context, svc *logging.Service, target string, start time.Time, extra string, limit int, seen map[string]bool) ([]types.LogEntry, error) {
	req := &logging.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + p.client.Project()},
		Filter:        entryFilter(p.client.Project(), target, start, extra),
		OrderBy:       "timestamp asc",
	}
	var entries []types.LogEntry
	err := svc.Entries.List(req).Pages(ctx, func(page *logging.ListLogEntriesResponse) error {
		for _, e := range page.Entries {
			if e.InsertId != "" {
				if seen[e.InsertId] {
					continue
				}
				seen[e.InsertId] = true
			}
			entries = append(entries, toLogEntry(e))
			if limit > 0 && len(entries) >= limit {
				return errPagesDone
			}
		}
		return nil
	})
	if errors.Is(err, errPagesDone) {
		err = nil
	}
	if err != nil {
		if isNotFound(err) {
			return nil, notFoundErr("log", target)
		}
		return nil, fmt.Errorf("failed to read log entries: %w", err)
	}
	return entries, nil
}

// resolveLogOpts applies the provider defaults for limit, since and filter.
func resolveLogOpts(now func() time.Time, opts *provider.LogsOptions) (limit int, since time.Time, filter string) {
	limit = defaultLogLimit
	if opts == nil {
		return
	}
	if opts.Limit > 0 {
		limit = opts.Limit
	}
	since = now().Add(-opts.Since)
	filter = opts.Filter
	return
}

// entryFilter builds the Logging query for one log: the log name, a
// timestamp floor and the user filter in parentheses.
func entryFilter(project, target string, start time.Time, extra string) string {
	filter := fmt.Sprintf(
		"logName=%q AND timestamp>=%q",
		logFullName(project, target),
		start.Format(time.RFC3339),
	)
	if extra != "" {
		filter += fmt.Sprintf(" AND (%s)", extra)
	}
	return filter
}

// logFullName builds the resource name of a log in the context project,
// URL-escaping the short name's slashes.
func logFullName(project, target string) string {
	return fmt.Sprintf("projects/%s/logs/%s", project, url.PathEscape(target))
}

// logShortName returns the URL-decoded last path segment of a log resource
// name, e.g. "projects/p/logs/x%2Fy" -> "x/y".
func logShortName(resource string) string {
	name := resource
	if i := strings.LastIndex(resource, "/"); i >= 0 {
		name = resource[i+1:]
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		return decoded
	}
	return name
}

// toLogEntry converts a Cloud Logging entry to the shared LogEntry shape.
func toLogEntry(e *logging.LogEntry) types.LogEntry {
	ts, err := time.Parse(time.RFC3339, e.Timestamp)
	if err != nil {
		ts = time.Time{}
	}
	return types.LogEntry{
		Timestamp: ts,
		Message:   entryMessage(e),
		Source:    logShortName(e.LogName),
		Level:     e.Severity,
		Provider:  providerGCPLogs,
	}
}

// entryMessage returns textPayload when set, else compact JSON of the
// jsonPayload or protoPayload.
func entryMessage(e *logging.LogEntry) string {
	if e.TextPayload != "" {
		return e.TextPayload
	}
	if len(e.JsonPayload) > 0 {
		return compactJSON(e.JsonPayload)
	}
	if len(e.ProtoPayload) > 0 {
		return compactJSON(e.ProtoPayload)
	}
	return ""
}

func compactJSON(raw googleapi.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// newestTimestamp returns the latest non-zero timestamp in entries.
func newestTimestamp(entries []types.LogEntry) time.Time {
	var newest time.Time
	for _, e := range entries {
		if e.Timestamp.After(newest) {
			newest = e.Timestamp
		}
	}
	return newest
}

// sleepCtx waits d, returning early when ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
