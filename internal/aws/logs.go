package aws

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// Defaults for log options when the caller passes zero values.
const (
	defaultLogsSince        = 15 * time.Minute
	defaultLogsLimit        = 100
	defaultLogsPollInterval = 2 * time.Second
)

// cwLogsAPI is the slice of the CloudWatch Logs API the provider uses. It
// mirrors the SDK's paginator client interfaces, so fakes can stand in for
// the real client in tests.
type cwLogsAPI interface {
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

// AWSLogsProvider implements provider.LogsProvider for CloudWatch Logs.
type AWSLogsProvider struct {
	client *Client
	api    cwLogsAPI // test seam; the lazily built SDK client is used when nil

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewLogsProvider creates a new CloudWatch Logs-backed LogsProvider.
func NewLogsProvider(client *Client) *AWSLogsProvider {
	return &AWSLogsProvider{
		client: client,
		now:    time.Now,
		sleep:  sleepWithContext,
	}
}

// apiClient returns the injected API, or builds the SDK client on first use.
func (p *AWSLogsProvider) apiClient() cwLogsAPI {
	if p.api != nil {
		return p.api
	}
	return p.client.CloudWatchLogs()
}

// ListTargets returns the log groups whose name starts with prefix.
func (p *AWSLogsProvider) ListTargets(ctx context.Context, prefix string) ([]types.LogTarget, error) {
	input := &cloudwatchlogs.DescribeLogGroupsInput{}
	if prefix != "" {
		input.LogGroupNamePrefix = aws.String(prefix)
	}

	var targets []types.LogTarget
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(p.apiClient(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list log groups: %w", err)
		}
		for _, g := range page.LogGroups {
			name := deref(g.LogGroupName)
			if name == "" {
				continue
			}
			t := types.LogTarget{Name: name, Provider: "aws"}
			if g.CreationTime != nil {
				t.CreatedAt = time.UnixMilli(*g.CreationTime)
			}
			if g.StoredBytes != nil {
				t.SizeBytes = *g.StoredBytes
			}
			targets = append(targets, t)
		}
	}
	return targets, nil
}

// Query returns the log entries from the last opts.Since, oldest first,
// at most opts.Limit (default 100), paginating as needed.
func (p *AWSLogsProvider) Query(ctx context.Context, target string, opts *provider.LogsOptions) ([]types.LogEntry, error) {
	o := normalizeLogsOptions(opts)
	start := p.now().Add(-o.Since)
	limit := o.Limit
	if limit <= 0 {
		limit = defaultLogsLimit
	}

	input := filterLogEventsInput(target, o.Filter, start)
	seen := make(map[string]struct{})
	return p.filterEvents(ctx, input, target, seen, limit)
}

// Tail emits the entries from the last opts.Since, then polls every
// opts.PollInterval for newer ones until ctx is cancelled or emit fails.
// Entries are deduped by event ID, so entries sharing a timestamp are still
// emitted exactly once.
func (p *AWSLogsProvider) Tail(ctx context.Context, target string, opts *provider.LogsOptions, emit func(types.LogEntry) error) error {
	o := normalizeLogsOptions(opts)
	start := p.now().Add(-o.Since)
	base := filterLogEventsInput(target, o.Filter, start)

	seen := make(map[string]struct{})
	var last time.Time

	for {
		if ctx.Err() != nil {
			return nil
		}
		// Resume from the newest event seen so far; entries at that exact
		// timestamp are refetched but deduped by ID.
		input := *base
		if !last.IsZero() {
			input.StartTime = aws.Int64(last.UnixMilli())
		}
		entries, err := p.filterEvents(ctx, &input, target, seen, 0)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Timestamp.After(last) {
				last = e.Timestamp
			}
			if err := emit(e); err != nil {
				return err
			}
		}
		if err := p.sleep(ctx, o.PollInterval); err != nil {
			return nil
		}
	}
}

// filterEvents pages through FilterLogEvents, deduping by event ID, and
// returns entries oldest-first. limit > 0 stops paging early once reached.
func (p *AWSLogsProvider) filterEvents(ctx context.Context, input *cloudwatchlogs.FilterLogEventsInput, target string, seen map[string]struct{}, limit int) ([]types.LogEntry, error) {
	var entries []types.LogEntry
	paginator := cloudwatchlogs.NewFilterLogEventsPaginator(p.apiClient(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, wrapLogEventErr(err, target)
		}
		for _, ev := range page.Events {
			id := deref(ev.EventId)
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			entries = append(entries, eventToEntry(ev))
		}
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	sortLogEntries(entries)
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// normalizeLogsOptions fills the provider defaults for zero/nil fields.
// Limit is intentionally left as-is: only Query applies a default to it.
func normalizeLogsOptions(opts *provider.LogsOptions) provider.LogsOptions {
	if opts == nil {
		return provider.LogsOptions{Since: defaultLogsSince, PollInterval: defaultLogsPollInterval}
	}
	o := *opts
	if o.Since <= 0 {
		o.Since = defaultLogsSince
	}
	if o.PollInterval <= 0 {
		o.PollInterval = defaultLogsPollInterval
	}
	return o
}

// filterLogEventsInput builds the FilterLogEvents input for a window.
func filterLogEventsInput(target, filter string, start time.Time) *cloudwatchlogs.FilterLogEventsInput {
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(target),
		StartTime:    aws.Int64(start.UnixMilli()),
	}
	if filter != "" {
		input.FilterPattern = aws.String(filter)
	}
	return input
}

// eventToEntry maps a CloudWatch event onto a LogEntry. CloudWatch events
// carry no severity, so Level stays empty.
func eventToEntry(ev cwltypes.FilteredLogEvent) types.LogEntry {
	e := types.LogEntry{
		Message:  deref(ev.Message),
		Source:   deref(ev.LogStreamName),
		Provider: "aws",
	}
	if ev.Timestamp != nil {
		e.Timestamp = time.UnixMilli(*ev.Timestamp)
	}
	return e
}

// wrapLogEventErr maps CloudWatch's not-found error onto a clear message and
// wraps everything else.
func wrapLogEventErr(err error, target string) error {
	var nf *cwltypes.ResourceNotFoundException
	if errors.As(err, &nf) {
		return fmt.Errorf("log group not found: %s", target)
	}
	return fmt.Errorf("failed to read log events from %s: %w", target, err)
}

// sortLogEntries orders entries oldest-first, with deterministic tie-breaks
// for events sharing a timestamp.
func sortLogEntries(entries []types.LogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].Timestamp.Before(entries[j].Timestamp)
		}
		if entries[i].Source != entries[j].Source {
			return entries[i].Source < entries[j].Source
		}
		return entries[i].Message < entries[j].Message
	})
}

// sleepWithContext waits d or until ctx is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
