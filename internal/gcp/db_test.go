package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"

	"github.com/vietdv277/cumulus/pkg/provider"
)

func TestParseDatabaseVersion(t *testing.T) {
	cases := []struct {
		databaseVersion string
		engine          string
		version         string
	}{
		{"POSTGRES_15", "postgres", "15"},
		{"POSTGRES_14", "postgres", "14"},
		{"MYSQL_8_0", "mysql", "8.0"},
		{"MYSQL_5_7", "mysql", "5.7"},
		{"SQLSERVER_2019_STANDARD", "sqlserver", "2019.standard"},
		{"SQL_DATABASE_VERSION_UNSPECIFIED", "sql", "database.version.unspecified"},
		{"", "", ""},
	}
	for _, tc := range cases {
		engine, version := parseDatabaseVersion(tc.databaseVersion)
		if engine != tc.engine || version != tc.version {
			t.Errorf("parseDatabaseVersion(%q) = (%q, %q), want (%q, %q)",
				tc.databaseVersion, engine, version, tc.engine, tc.version)
		}
	}
}

func TestDefaultDBPort(t *testing.T) {
	cases := []struct {
		engine string
		port   int
		err    bool
	}{
		{"postgres", 5432, false},
		{"mysql", 3306, false},
		{"sqlserver", 1433, false},
		{"mongodb", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		port, err := defaultDBPort(tc.engine)
		if tc.err && err == nil {
			t.Errorf("defaultDBPort(%q) = %d, want error", tc.engine, port)
			continue
		}
		if !tc.err {
			if err != nil {
				t.Errorf("defaultDBPort(%q) error = %v, want nil", tc.engine, err)
				continue
			}
			if port != tc.port {
				t.Errorf("defaultDBPort(%q) = %d, want %d", tc.engine, port, tc.port)
			}
		}
	}
}

func TestCloudSQLState(t *testing.T) {
	cases := map[string]string{
		"RUNNABLE":       "available",
		"SUSPENDED":      "stopped",
		"PENDING_CREATE": "creating",
		"PENDING_DELETE": "deleting",
		"MAINTENANCE":    "maintenance",
		"FAILED":         "failed",
		"WHAT":           "what",
	}
	for in, want := range cases {
		if got := cloudSQLState(in); got != want {
			t.Errorf("cloudSQLState(%q) = %q, want %q", in, got, want)
		}
	}
}

// newTestDBProvider returns a provider whose SQL Admin client talks to srv.
func newTestDBProvider(t *testing.T, project string, handler http.HandlerFunc) *GCPDBProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := sqladmin.NewService(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("building test SQL Admin service: %v", err)
	}
	p := NewDBProvider(&Client{project: project})
	p.sql = svc
	return p
}

func TestGCPDBListPagination(t *testing.T) {
	calls := 0
	p := newTestDBProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"items":[`+
				`{"name":"pg-prod","databaseVersion":"POSTGRES_15","state":"RUNNABLE",`+
				`"connectionName":"proj:us-central1:pg-prod","createTime":"2024-01-02T03:04:05Z",`+
				`"settings":{"tier":"db-custom-2-7680"}}],`+
				`"nextPageToken":"tok2"}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"name":"mysql-dev","databaseVersion":"MYSQL_8_0","state":"SUSPENDED",`+
			`"connectionName":"proj:europe-west1:mysql-dev"}]}`)
	})

	dbs, err := p.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(dbs) != 2 {
		t.Fatalf("List() returned %d databases, want 2", len(dbs))
	}

	pg := dbs[0]
	if pg.Engine != "postgres" || pg.Version != "15" {
		t.Errorf("postgres engine/version = %q/%q, want postgres/15", pg.Engine, pg.Version)
	}
	if pg.Endpoint != "proj:us-central1:pg-prod" {
		t.Errorf("Endpoint = %q, want the connection name", pg.Endpoint)
	}
	if pg.Port != 5432 {
		t.Errorf("Port = %d, want 5432 (postgres default)", pg.Port)
	}
	if pg.State != "available" {
		t.Errorf("State = %q, want available", pg.State)
	}
	if pg.Size != "db-custom-2-7680" {
		t.Errorf("Size = %q, want instance tier", pg.Size)
	}
	if pg.CreatedAt.Format("2006-01-02") != "2024-01-02" {
		t.Errorf("CreatedAt = %v, want 2024-01-02", pg.CreatedAt)
	}
	if pg.Provider != "gcp" {
		t.Errorf("Provider = %q, want gcp", pg.Provider)
	}

	my := dbs[1]
	if my.Engine != "mysql" || my.Version != "8.0" {
		t.Errorf("mysql engine/version = %q/%q, want mysql/8.0", my.Engine, my.Version)
	}
	if my.Port != 3306 {
		t.Errorf("Port = %d, want 3306 (mysql default)", my.Port)
	}
	if my.State != "stopped" {
		t.Errorf("State = %q, want stopped", my.State)
	}

	if calls != 2 {
		t.Errorf("server got %d list calls, want 2 (one per page)", calls)
	}
}

func TestGCPDBListEngineFilter(t *testing.T) {
	p := newTestDBProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[`+
			`{"name":"pg-prod","databaseVersion":"POSTGRES_15","state":"RUNNABLE"},`+
			`{"name":"mysql-dev","databaseVersion":"MYSQL_8_0","state":"RUNNABLE"}]}`)
	})

	dbs, err := p.List(context.Background(), &provider.DBFilter{Engine: "postgres"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(dbs) != 1 || dbs[0].Name != "pg-prod" {
		t.Errorf("List(engine filter) = %+v, want only pg-prod", dbs)
	}
}

func TestGCPDBGet(t *testing.T) {
	p := newTestDBProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/instances/prod-pg") {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"name":"prod-pg","databaseVersion":"POSTGRES_15","state":"RUNNABLE",`+
			`"connectionName":"proj:us-central1:prod-pg"}`)
	})

	db, err := p.Get(context.Background(), "prod-pg")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if db.Name != "prod-pg" || db.Engine != "postgres" || db.Version != "15" {
		t.Errorf("Get() = %+v, want prod-pg postgres 15", db)
	}
}

func TestGCPDBGetNotFound(t *testing.T) {
	p := newTestDBProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
	})

	if _, err := p.Get(context.Background(), "missing"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Get() error = %v, want provider.ErrNotFound", err)
	}
}

func TestGCPDBGetOtherErrorsWrapped(t *testing.T) {
	p := newTestDBProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})

	if _, err := p.Get(context.Background(), "x"); err == nil || errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Get() error = %v, want a wrapped non-not-found error", err)
	}
}
