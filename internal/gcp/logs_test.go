package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/logging/v2"
	"google.golang.org/api/option"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// capturedRequest captures the API calls a test server receives, including
// the raw body (entries:list is a POST).
type capturedRequest struct {
	method string
	path   string
	query  string
	body   string
}

// newTestLogsProvider returns a provider whose Cloud Logging client talks to
// srv, recording every request into reqs.
func newTestLogsProvider(t *testing.T, project string, handler http.HandlerFunc) (*GCPLogsProvider, *[]capturedRequest) {
	t.Helper()
	reqs := &[]capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := logging.NewService(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("building test Cloud Logging service: %v", err)
	}
	p := NewLogsProvider(&Client{project: project})
	p.svc = svc
	return p, reqs
}

// decodeEntriesRequest unmarshals a captured request body into the SDK type.
func decodeEntriesRequest(t *testing.T, req capturedRequest) logging.ListLogEntriesRequest {
	t.Helper()
	var reqBody logging.ListLogEntriesRequest
	if err := json.Unmarshal([]byte(req.body), &reqBody); err != nil {
		t.Fatalf("decoding entries:list request body %q: %v", req.body, err)
	}
	return reqBody
}

func TestEntryFilter(t *testing.T) {
	start := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	ns := time.Date(2024, 5, 6, 7, 8, 9, 123456789, time.UTC)
	for _, tt := range []struct {
		name   string
		target string
		start  time.Time
		extra  string
		want   string
	}{
		{
			name:   "escapes slashes in the log name",
			target: "app/web",
			start:  start,
			want:   `logName="projects/proj/logs/app%2Fweb" AND timestamp>="2024-05-06T07:08:09Z"`,
		},
		{
			name:   "no user filter",
			target: "app/web",
			start:  start,
			want:   `logName="projects/proj/logs/app%2Fweb" AND timestamp>="2024-05-06T07:08:09Z"`,
		},
		{
			name:   "parenthesises the user filter",
			target: "app/web",
			start:  start,
			extra:  `severity="ERROR"`,
			want:   `logName="projects/proj/logs/app%2Fweb" AND timestamp>="2024-05-06T07:08:09Z" AND (severity="ERROR")`,
		},
		{
			name:   "truncates the timestamp floor to whole seconds",
			target: "syslog",
			start:  ns,
			want:   `logName="projects/proj/logs/syslog" AND timestamp>="2024-05-06T07:08:09Z"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := entryFilter("proj", tt.target, tt.start, tt.extra)
			if got != tt.want {
				t.Errorf("entryFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGCPLogsQueryFilterString(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	p, reqs := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	p.now = func() time.Time { return now }

	if _, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{
		Since:  2 * time.Hour,
		Filter: `severity="ERROR"`,
	}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(*reqs) != 1 || (*reqs)[0].method != "POST" || (*reqs)[0].path != "/v2/entries:list" {
		t.Fatalf("request = %+v, want POST /v2/entries:list", *reqs)
	}
	body := decodeEntriesRequest(t, (*reqs)[0])
	if len(body.ResourceNames) != 1 || body.ResourceNames[0] != "projects/proj" {
		t.Errorf("resourceNames = %v, want [projects/proj]", body.ResourceNames)
	}
	if body.OrderBy != "timestamp asc" {
		t.Errorf("orderBy = %q, want timestamp asc", body.OrderBy)
	}
	want := `logName="projects/proj/logs/app%2Fweb" AND timestamp>="2024-01-02T01:04:05Z" AND (severity="ERROR")`
	if body.Filter != want {
		t.Errorf("filter = %q, want %q", body.Filter, want)
	}
}

func TestGCPLogsQueryPagination(t *testing.T) {
	calls := 0
	p, reqs := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"entries":[{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"}],"nextPageToken":"tok2"}`)
			return
		}
		fmt.Fprint(w, `{"entries":[{"insertId":"b","timestamp":"2024-01-02T03:04:06Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"}]}`)
	})

	entries, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Query() returned %d entries, want 2", len(entries))
	}
	if entries[0].Message != "one" || entries[1].Message != "two" {
		t.Errorf("messages = %q, %q; want one, two (oldest first)", entries[0].Message, entries[1].Message)
	}
	if calls != 2 {
		t.Errorf("server got %d calls, want 2 (one per page)", calls)
	}
	if len(*reqs) != 2 {
		t.Fatalf("server got %d requests, want 2", len(*reqs))
	}
	// entries:list is a POST: the page token travels in the body.
	second := decodeEntriesRequest(t, (*reqs)[1])
	if second.PageToken != "tok2" {
		t.Errorf("second request pageToken = %q, want tok2", second.PageToken)
	}
}

func TestGCPLogsQueryStopsAtLimit(t *testing.T) {
	p, reqs := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entries":[`+
			`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"},`+
			`{"insertId":"b","timestamp":"2024-01-02T03:04:06Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"},`+
			`{"insertId":"c","timestamp":"2024-01-02T03:04:07Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"three"}],`+
			`"nextPageToken":"more"}`)
	})

	entries, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour, Limit: 2})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(entries) != 2 || entries[1].Message != "two" {
		t.Errorf("Query() returned %v, want the first 2 entries", entries)
	}
	if len(*reqs) != 1 {
		t.Errorf("server got %d requests, want 1 (no fetch past the limit)", len(*reqs))
	}
}

func TestGCPLogsQueryDedupesByInsertId(t *testing.T) {
	p, _ := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entries":[`+
			`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"},`+
			`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"},`+
			`{"insertId":"b","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"}]}`)
	})

	entries, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Query() returned %d entries, want 2 after dedupe", len(entries))
	}
}

func TestGCPLogsQueryPayloadFallbacks(t *testing.T) {
	p, _ := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entries":[`+
			`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","severity":"ERROR","logName":"projects/proj/logs/app%2Fweb","textPayload":"boom"},`+
			`{"insertId":"b","timestamp":"2024-01-02T03:04:05.25Z","logName":"projects/proj/logs/app%2Fweb","jsonPayload":{"msg":"hello","n":1}},`+
			`{"insertId":"c","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","protoPayload":{"@type":"type.googleapis.com/google.cloud.audit.AuditLog"}},`+
			`{"insertId":"d","logName":"projects/proj/logs/app%2Fweb"}]}`)
	})

	entries, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	wantMessages := []string{
		"boom",
		`{"msg":"hello","n":1}`,
		`{"@type":"type.googleapis.com/google.cloud.audit.AuditLog"}`,
		"",
	}
	for i, want := range wantMessages {
		if entries[i].Message != want {
			t.Errorf("entry %d message = %q, want %q", i, entries[i].Message, want)
		}
	}
	if entries[0].Level != "ERROR" {
		t.Errorf("level = %q, want ERROR (severity)", entries[0].Level)
	}
	if entries[0].Source != "app/web" || entries[0].Provider != "gcp" {
		t.Errorf("source/provider = %q/%q, want app/web/gcp (URL-decoded log name)", entries[0].Source, entries[0].Provider)
	}
	if got := entries[1].Timestamp.Format(time.RFC3339Nano); got != "2024-01-02T03:04:05.25Z" {
		t.Errorf("timestamp = %q, want 2024-01-02T03:04:05.25Z", got)
	}
}

func TestGCPLogsQueryNotFound(t *testing.T) {
	p, _ := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
	})

	err := func() error {
		_, err := p.Query(context.Background(), "missing", &provider.LogsOptions{Since: time.Hour})
		return err
	}()
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Query() error = %v, want provider.ErrNotFound", err)
	}
	if err != nil && !strings.Contains(err.Error(), `"missing"`) {
		t.Errorf("Query() error = %v, want the target name quoted", err)
	}
}

func TestGCPLogsQueryOtherErrorsWrapped(t *testing.T) {
	p, _ := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})

	_, err := p.Query(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour})
	if err == nil {
		t.Fatal("Query() = nil error, want wrapped 403")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Query() error = %v, must not be classified not-found", err)
	}
}

func TestGCPLogsListTargets(t *testing.T) {
	calls := 0
	p, reqs := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v2/projects/proj/logs" {
			t.Errorf("request path = %q, want /v2/projects/proj/logs", r.URL.Path)
		}
		if calls == 1 {
			fmt.Fprint(w, `{"logNames":["projects/proj/logs/app%2Fweb","projects/proj/logs/app%2Fdb"],"nextPageToken":"tok2"}`)
			return
		}
		fmt.Fprint(w, `{"logNames":["projects/proj/logs/other"]}`)
	})

	targets, err := p.ListTargets(context.Background(), "app/")
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}

	var names []string
	for _, tgt := range targets {
		names = append(names, tgt.Name)
		if tgt.Provider != "gcp" {
			t.Errorf("target %q provider = %q, want gcp", tgt.Name, tgt.Provider)
		}
	}
	if len(names) != 2 || names[0] != "app/web" || names[1] != "app/db" {
		t.Errorf("names = %v, want [app/web app/db] (URL-decoded, prefix-matched)", names)
	}
	if calls != 2 {
		t.Errorf("server got %d calls, want 2 (one per page)", calls)
	}
	if len(*reqs) != 2 || !strings.Contains((*reqs)[1].query, "pageToken=tok2") {
		t.Errorf("second request query = %q, want pageToken=tok2", (*reqs)[1].query)
	}
}

func TestGCPLogsTailDedupeAndCancel(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	calls := 0
	p, reqs := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1: // initial window: two entries sharing one timestamp
			fmt.Fprint(w, `{"entries":[`+
				`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"},`+
				`{"insertId":"b","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"}]}`)
		case 2: // first poll: b again (same insertId) plus a new entry, same timestamp
			fmt.Fprint(w, `{"entries":[`+
				`{"insertId":"b","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"},`+
				`{"insertId":"c","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"three"}]}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
	p.now = func() time.Time { return now }

	sleeps := 0
	p.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps++
		if sleeps == 1 {
			return nil // first poll happens immediately
		}
		<-ctx.Done()
		return ctx.Err() // second poll blocks until the test cancels
	}

	var got []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := p.Tail(ctx, "app/web", &provider.LogsOptions{Since: time.Hour, PollInterval: time.Hour},
		func(e types.LogEntry) error {
			got = append(got, e.Message)
			if e.Message == "three" {
				cancel()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Tail() error = %v, want clean nil on cancellation", err)
	}

	want := []string{"one", "two", "three"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emitted = %v, want %v", got, want)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("emitted = %v, want exactly %v (b never emitted twice, despite shared timestamp)", got, want)
	}
	if len(*reqs) != 2 {
		t.Errorf("server got %d requests, want 2 (initial window + one poll)", len(*reqs))
	}
	// The poll must re-query from the newest initial timestamp, so entries
	// sharing it but with a new insertId are still returned.
	body := decodeEntriesRequest(t, (*reqs)[1])
	wantPollFilter := `logName="projects/proj/logs/app%2Fweb" AND timestamp>="2024-01-02T03:04:05Z"`
	if body.Filter != wantPollFilter {
		t.Errorf("poll filter = %q, want %q", body.Filter, wantPollFilter)
	}
}

func TestGCPLogsTailReturnsEmitError(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	p, _ := newTestLogsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entries":[`+
			`{"insertId":"a","timestamp":"2024-01-02T03:04:05Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"one"},`+
			`{"insertId":"b","timestamp":"2024-01-02T03:04:06Z","logName":"projects/proj/logs/app%2Fweb","textPayload":"two"}]}`)
	})
	p.now = func() time.Time { return now }

	sleeps := 0
	p.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps++
		return nil
	}

	emitErr := errors.New("emit failed")
	err := p.Tail(context.Background(), "app/web", &provider.LogsOptions{Since: time.Hour},
		func(e types.LogEntry) error {
			if e.Message == "two" {
				return emitErr
			}
			return nil
		})
	if !errors.Is(err, emitErr) {
		t.Errorf("Tail() error = %v, want emit's error", err)
	}
	if sleeps != 0 {
		t.Errorf("Tail slept %d times before returning, want 0", sleeps)
	}
}
