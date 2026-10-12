package aws

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// fakeLogsAPI serves one canned page per call and records each request, so
// tests can drive Query/ListTargets/Tail without touching AWS.
type fakeLogsAPI struct {
	groupPages [][]cwltypes.LogGroup
	eventPages [][]cwltypes.FilteredLogEvent
	err        error // returned by FilterLogEvents on every call when set

	describeCalls []cloudwatchlogs.DescribeLogGroupsInput
	filterCalls   []cloudwatchlogs.FilterLogEventsInput
}

func (f *fakeLogsAPI) DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	f.describeCalls = append(f.describeCalls, *params)
	if len(f.groupPages) == 0 {
		return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
	}
	page := f.groupPages[0]
	f.groupPages = f.groupPages[1:]
	out := &cloudwatchlogs.DescribeLogGroupsOutput{LogGroups: page}
	if len(f.groupPages) > 0 {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

func (f *fakeLogsAPI) FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	f.filterCalls = append(f.filterCalls, *params)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.eventPages) == 0 {
		return &cloudwatchlogs.FilterLogEventsOutput{}, nil
	}
	page := f.eventPages[0]
	f.eventPages = f.eventPages[1:]
	out := &cloudwatchlogs.FilterLogEventsOutput{Events: page}
	if len(f.eventPages) > 0 {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

// logEvent builds a FilterLogEvent with the given id/stream/timestamp.
func logEvent(id, stream string, ts time.Time, msg string) cwltypes.FilteredLogEvent {
	return cwltypes.FilteredLogEvent{
		EventId:       aws.String(id),
		LogStreamName: aws.String(stream),
		Timestamp:     aws.Int64(ts.UnixMilli()),
		Message:       aws.String(msg),
	}
}

// newTestLogsProvider wires a provider to a fake API with a fixed clock.
func newTestLogsProvider(fake *fakeLogsAPI, now time.Time) *AWSLogsProvider {
	return &AWSLogsProvider{
		api:   fake,
		now:   func() time.Time { return now },
		sleep: sleepWithContext,
	}
}

func TestLogsQueryLimitOrderingAndDedupe(t *testing.T) {
	base := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	fake := &fakeLogsAPI{eventPages: [][]cwltypes.FilteredLogEvent{
		{
			logEvent("e3", "s1", base.Add(3*time.Second), "newest"),
			logEvent("e1", "s1", base, "first"),
			logEvent("e2", "s2", base, "tie on ts"),
		},
		{
			logEvent("e1", "s1", base, "first"), // duplicate across pages
			logEvent("e4", "s1", base.Add(2*time.Second), "second newest"),
		},
	}}
	p := newTestLogsProvider(fake, base.Add(time.Minute))

	entries, err := p.Query(context.Background(), "grp", &provider.LogsOptions{Since: 10 * time.Minute, Limit: 4})
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}

	// Oldest first, deduped by event ID.
	want := []string{"first", "tie on ts", "second newest", "newest"}
	if len(entries) != len(want) {
		t.Fatalf("Query() = %d entries, want %d", len(entries), len(want))
	}
	for i, w := range want {
		if entries[i].Message != w {
			t.Errorf("entries[%d].Message = %q, want %q", i, entries[i].Message, w)
		}
	}
	if !entries[0].Timestamp.Equal(base) {
		t.Errorf("entries[0].Timestamp = %v, want %v", entries[0].Timestamp, base)
	}
	if entries[0].Provider != "aws" || entries[0].Source != "s1" || entries[0].Level != "" {
		t.Errorf("entries[0] = %+v, want provider aws, source s1, empty level", entries[0])
	}

	// The window start is now-Since in milliseconds (now = base+1m).
	if got, want := aws.ToInt64(fake.filterCalls[0].StartTime), base.Add(-9*time.Minute).UnixMilli(); got != want {
		t.Errorf("StartTime = %d, want %d", got, want)
	}
	// Both pages were fetched to fill the limit, deduping e1 across pages.
	if len(fake.filterCalls) != 2 {
		t.Errorf("FilterLogEvents calls = %d, want 2", len(fake.filterCalls))
	}
}

func TestLogsQueryDefaultsAndFilter(t *testing.T) {
	base := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	pattern := `{ $.level = "error" }`
	fake := &fakeLogsAPI{}
	p := newTestLogsProvider(fake, base)

	if _, err := p.Query(context.Background(), "grp", &provider.LogsOptions{Filter: pattern}); err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}

	call := fake.filterCalls[0]
	if got, want := aws.ToInt64(call.StartTime), base.Add(-defaultLogsSince).UnixMilli(); got != want {
		t.Errorf("default StartTime = %d, want %d (now-15m)", got, want)
	}
	if aws.ToString(call.FilterPattern) != pattern {
		t.Errorf("FilterPattern = %q, want %q", aws.ToString(call.FilterPattern), pattern)
	}

	// More than the default limit of 100 events must truncate to 100.
	events := make([]cwltypes.FilteredLogEvent, 0, 150)
	for i := 0; i < 150; i++ {
		events = append(events, logEvent(string(rune('a'+i%26))+string(rune('a'+i/26)), "s", base.Add(time.Duration(i)*time.Second), "m"))
	}
	fake.eventPages = append(fake.eventPages, events[:75], events[75:])

	entries, err := p.Query(context.Background(), "grp", nil)
	if err != nil {
		t.Fatalf("Query(nil opts) unexpected error: %v", err)
	}
	if len(entries) != defaultLogsLimit {
		t.Errorf("Query(nil opts) = %d entries, want %d", len(entries), defaultLogsLimit)
	}
}

func TestLogsQueryNotFound(t *testing.T) {
	fake := &fakeLogsAPI{err: &cwltypes.ResourceNotFoundException{}}
	p := newTestLogsProvider(fake, time.Now())

	_, err := p.Query(context.Background(), "grp", nil)
	if err == nil || err.Error() != "log group not found: grp" {
		t.Fatalf("Query() error = %v, want %q", err, "log group not found: grp")
	}
}

func TestLogsQueryWrappedError(t *testing.T) {
	fake := &fakeLogsAPI{err: errors.New("access denied")}
	p := newTestLogsProvider(fake, time.Now())

	_, err := p.Query(context.Background(), "grp", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to read log events from grp") || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("Query() error = %v, want wrapped access denied", err)
	}
}

func TestLogsListTargets(t *testing.T) {
	fake := &fakeLogsAPI{groupPages: [][]cwltypes.LogGroup{
		{
			{LogGroupName: aws.String("/aws/lambda/api"), CreationTime: aws.Int64(1700000000000), StoredBytes: aws.Int64(2048)},
			{LogGroupName: aws.String("/aws/lambda/worker")},
		},
		{{LogGroupName: aws.String("/aws/lambda/other")}},
	}}
	p := newTestLogsProvider(fake, time.Now())

	targets, err := p.ListTargets(context.Background(), "/aws/lambda/")
	if err != nil {
		t.Fatalf("ListTargets() unexpected error: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("ListTargets() = %d targets, want 3", len(targets))
	}
	first := targets[0]
	if first.Name != "/aws/lambda/api" || first.Provider != "aws" {
		t.Errorf("targets[0] = %+v, want /aws/lambda/api on aws", first)
	}
	if first.CreatedAt.UnixMilli() != 1700000000000 || first.SizeBytes != 2048 {
		t.Errorf("targets[0] = %+v, want created 1700000000000 and 2048 bytes", first)
	}
	if !targets[1].CreatedAt.IsZero() || targets[1].SizeBytes != 0 {
		t.Errorf("targets[1] = %+v, want zero created/size", targets[1])
	}
	if aws.ToString(fake.describeCalls[0].LogGroupNamePrefix) != "/aws/lambda/" {
		t.Errorf("LogGroupNamePrefix = %q, want %q", aws.ToString(fake.describeCalls[0].LogGroupNamePrefix), "/aws/lambda/")
	}
}

func TestLogsTailDedupeAndCancellation(t *testing.T) {
	base := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	fake := &fakeLogsAPI{eventPages: [][]cwltypes.FilteredLogEvent{
		{
			logEvent("e1", "s1", base, "one"),
			logEvent("e2", "s1", base, "two"), // same timestamp as e1
		},
		{
			logEvent("e2", "s1", base, "two"), // re-served on the next poll
			logEvent("e3", "s1", base.Add(time.Second), "three"),
		},
	}}
	p := newTestLogsProvider(fake, base)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	polls := 0
	p.sleep = func(c context.Context, d time.Duration) error {
		polls++
		if polls == 2 {
			cancel()
			return context.Canceled
		}
		return nil
	}

	var emitted []types.LogEntry
	err := p.Tail(ctx, "grp", &provider.LogsOptions{Since: time.Minute, PollInterval: 10 * time.Millisecond},
		func(e types.LogEntry) error { emitted = append(emitted, e); return nil })
	if err != nil {
		t.Fatalf("Tail() unexpected error: %v", err)
	}

	want := []string{"one", "two", "three"}
	if len(emitted) != len(want) {
		t.Fatalf("Tail() emitted %d entries (%v), want %v", len(emitted), emitted, want)
	}
	for i, w := range want {
		if emitted[i].Message != w {
			t.Errorf("emitted[%d] = %q, want %q", i, emitted[i].Message, w)
		}
	}

	// The poll after the initial window resumes from the newest event's
	// timestamp (base+1s), not the window start. The initial window itself
	// consumed two pages (both carry the original StartTime).
	if len(fake.filterCalls) < 3 {
		t.Fatalf("FilterLogEvents calls = %d, want at least 3", len(fake.filterCalls))
	}
	if got, want := aws.ToInt64(fake.filterCalls[2].StartTime), base.Add(time.Second).UnixMilli(); got != want {
		t.Errorf("poll after window StartTime = %d, want %d", got, want)
	}
}

func TestLogsTailEmitErrorPropagates(t *testing.T) {
	base := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	fake := &fakeLogsAPI{eventPages: [][]cwltypes.FilteredLogEvent{
		{logEvent("e1", "s1", base, "one")},
	}}
	p := newTestLogsProvider(fake, base)

	boom := errors.New("emit boom")
	err := p.Tail(context.Background(), "grp", &provider.LogsOptions{Since: time.Minute},
		func(e types.LogEntry) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Tail() error = %v, want emit boom", err)
	}
	if len(fake.filterCalls) != 1 {
		t.Errorf("FilterLogEvents calls = %d, want 1 (tail must stop)", len(fake.filterCalls))
	}
}

func TestLogsTailCancelledBeforeStart(t *testing.T) {
	p := newTestLogsProvider(&fakeLogsAPI{}, time.Now())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Tail(ctx, "grp", nil, func(e types.LogEntry) error { return nil })
	if err != nil {
		t.Fatalf("Tail() on cancelled ctx = %v, want nil", err)
	}
}

func TestNormalizeLogsOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     *provider.LogsOptions
		want     provider.LogsOptions
		wantIter int
	}{
		{
			name: "nil gets defaults",
			opts: nil,
			want: provider.LogsOptions{Since: defaultLogsSince, PollInterval: defaultLogsPollInterval},
		},
		{
			name: "zeros get defaults",
			opts: &provider.LogsOptions{},
			want: provider.LogsOptions{Since: defaultLogsSince, PollInterval: defaultLogsPollInterval},
		},
		{
			name: "explicit values kept",
			opts: &provider.LogsOptions{Since: time.Hour, PollInterval: 5 * time.Second, Limit: 7},
			want: provider.LogsOptions{Since: time.Hour, PollInterval: 5 * time.Second, Limit: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLogsOptions(tt.opts); got != tt.want {
				t.Errorf("normalizeLogsOptions(%v) = %+v, want %+v", tt.opts, got, tt.want)
			}
		})
	}
}
