package gcp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"

	"github.com/vietdv277/cumulus/pkg/provider"
)

// recordedRequest captures the API calls a test server receives.
type recordedRequest struct {
	method string
	path   string
	query  string
}

// newTestSecretsProvider returns a provider whose Secret Manager client talks
// to srv, recording every request into reqs.
func newTestSecretsProvider(t *testing.T, project string, handler http.HandlerFunc) (*GCPSecretsProvider, *[]recordedRequest) {
	t.Helper()
	reqs := &[]recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reqs = append(*reqs, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := secretmanager.NewService(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("building test Secret Manager service: %v", err)
	}
	p := NewSecretsProvider(&Client{project: project})
	p.sm = svc
	return p, reqs
}

func TestGCPSecretsListPagination(t *testing.T) {
	calls := 0
	p, reqs := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"secrets":[{"name":"projects/proj/secrets/first","createTime":"2024-01-02T03:04:05Z"}],"nextPageToken":"tok2"}`)
			return
		}
		fmt.Fprint(w, `{"secrets":[{"name":"projects/proj/secrets/second"}]}`)
	})

	secrets, err := p.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(secrets) != 2 {
		t.Fatalf("List() returned %d secrets, want 2", len(secrets))
	}
	if secrets[0].Name != "first" || secrets[1].Name != "second" {
		t.Errorf("names = %q, %q; want first, second", secrets[0].Name, secrets[1].Name)
	}
	if secrets[0].Provider != "gcp" {
		t.Errorf("Provider = %q, want gcp", secrets[0].Provider)
	}
	if secrets[0].ARN != "projects/proj/secrets/first" {
		t.Errorf("ARN = %q, want full resource name", secrets[0].ARN)
	}
	if got := secrets[0].CreatedAt.Format("2006-01-02"); got != "2024-01-02" {
		t.Errorf("CreatedAt = %q, want 2024-01-02", got)
	}
	if calls != 2 {
		t.Errorf("server got %d list calls, want 2 (one per page)", calls)
	}
	if len(*reqs) != 2 || !strings.Contains((*reqs)[1].query, "pageToken=tok2") {
		t.Errorf("second request query = %q, want pageToken=tok2", (*reqs)[1].query)
	}
}

func TestGCPSecretsListPrefix(t *testing.T) {
	p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"secrets":[{"name":"projects/proj/secrets/app-key"},{"name":"projects/proj/secrets/db-pass"}]}`)
	})

	secrets, err := p.List(context.Background(), &provider.SecretFilter{Prefix: "db-"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(secrets) != 1 || secrets[0].Name != "db-pass" {
		t.Errorf("List() with prefix returned %+v, want only db-pass", secrets)
	}
}

func TestGCPSecretsListRejectsAWSSource(t *testing.T) {
	for _, source := range []provider.SecretSource{provider.SourceSSM, provider.SourceSM} {
		p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {})
		if _, err := p.List(context.Background(), &provider.SecretFilter{Source: source}); err == nil {
			t.Errorf("List(Source=%s) = nil error, want error", source)
		}
	}
}

func TestGCPSecretsGetDecodesPayload(t *testing.T) {
	p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/versions/latest:access") {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		fmt.Fprintf(w, `{"name":"projects/proj/secrets/my-secret/versions/3","payload":{"data":%q}}`,
			base64.StdEncoding.EncodeToString([]byte("hunter2")))
	})

	sv, err := p.Get(context.Background(), "my-secret")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if sv.Value != "hunter2" {
		t.Errorf("Value = %q, want hunter2 (decoded)", sv.Value)
	}
	if sv.Version != "3" {
		t.Errorf("Version = %q, want 3", sv.Version)
	}
	if sv.Name != "my-secret" || sv.Provider != "gcp" {
		t.Errorf("Name/Provider = %q/%q, want my-secret/gcp", sv.Name, sv.Provider)
	}
}

func TestGCPSecretsGetNotFound(t *testing.T) {
	p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
	})

	if _, err := p.Get(context.Background(), "missing"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Get() error = %v, want provider.ErrNotFound", err)
	}
}

// TestGCPSecretsGetAbortsOnCancelledContext proves Get propagates ctx to the
// API call: the server holds the request in flight, so only cancellation can
// unblock it.
func TestGCPSecretsGetAbortsOnCancelledContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() { close(release); srv.Close() }()

	svc, err := secretmanager.NewService(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("building test Secret Manager service: %v", err)
	}
	p := NewSecretsProvider(&Client{project: "proj"})
	p.sm = svc

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := p.Get(ctx, "my-secret")
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Get() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get did not return after context cancellation")
	}
}

func TestGCPSecretsSetExisting(t *testing.T) {
	p, reqs := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})

	if err := p.Set(context.Background(), "my-secret", "v1"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("server got %d requests, want 1 (no create)", len(*reqs))
	}
	if (*reqs)[0].method != "POST" || !strings.HasSuffix((*reqs)[0].path, "/secrets/my-secret:addVersion") {
		t.Errorf("request = %v %v, want POST .../addVersion", (*reqs)[0].method, (*reqs)[0].path)
	}
}

func TestGCPSecretsSetCreatesOn404Only(t *testing.T) {
	addVersionCalls := 0
	created := ""
	p, reqs := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":addVersion"):
			addVersionCalls++
			if addVersionCalls == 1 {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
				return
			}
			fmt.Fprint(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/secrets"):
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			created = r.FormValue("secretId")
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	if err := p.Set(context.Background(), "new-secret", "v1"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if created != "new-secret" {
		t.Errorf("create secretId = %q, want new-secret", created)
	}
	if addVersionCalls != 2 {
		t.Errorf("addVersion called %d times, want 2 (404 then retry)", addVersionCalls)
	}
	// Order matters: the create must sit between the two version adds.
	if len(*reqs) != 3 ||
		!strings.HasSuffix((*reqs)[0].path, ":addVersion") ||
		!strings.HasSuffix((*reqs)[1].path, "/secrets") ||
		!strings.HasSuffix((*reqs)[2].path, ":addVersion") {
		t.Errorf("request order = %v, want addVersion, create, addVersion", *reqs)
	}
}

func TestGCPSecretsSetOtherErrorsWrapped(t *testing.T) {
	p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})

	err := p.Set(context.Background(), "my-secret", "v1")
	if err == nil {
		t.Fatal("Set() = nil error, want wrapped 403")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Set() error = %v, must not be classified not-found", err)
	}
}

func TestGCPSecretsDelete(t *testing.T) {
	p, reqs := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})

	if err := p.Delete(context.Background(), "old", provider.DeleteSecretOptions{Force: true}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(*reqs) != 1 || (*reqs)[0].method != "DELETE" || !strings.HasSuffix((*reqs)[0].path, "/secrets/old") {
		t.Errorf("request = %+v, want DELETE .../secrets/old", *reqs)
	}
}

func TestGCPSecretsDeleteNotFound(t *testing.T) {
	p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
	})

	if err := p.Delete(context.Background(), "missing", provider.DeleteSecretOptions{}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("Delete() error = %v, want provider.ErrNotFound", err)
	}
}

func TestGCPSecretsRejectsBackendPrefixes(t *testing.T) {
	for _, name := range []string{"ssm:db-pass", "sm:db-pass"} {
		p, _ := newTestSecretsProvider(t, "proj", func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected API call %s %s for prefixed name", r.Method, r.URL.Path)
		})
		for _, act := range []func() error{
			func() error { _, err := p.Get(context.Background(), name); return err },
			func() error { return p.Set(context.Background(), name, "v") },
			func() error { return p.Delete(context.Background(), name, provider.DeleteSecretOptions{}) },
		} {
			if err := act(); err == nil || !strings.Contains(err.Error(), "AWS-only") {
				t.Errorf("GCP op on %q error = %v, want AWS-only prefix error", name, err)
			}
		}
	}
}

func TestIsNotFound(t *testing.T) {
	gerr := &googleapi.Error{Code: 404}
	if !isNotFound(gerr) {
		t.Error("isNotFound(404 googleapi.Error) = false, want true")
	}
	if isNotFound(&googleapi.Error{Code: 403}) {
		t.Error("isNotFound(403 googleapi.Error) = true, want false")
	}
	if isNotFound(errors.New("connection refused")) {
		t.Error("isNotFound(plain error) = true, want false")
	}
	if !errors.Is(fmt.Errorf("x: %w", gerr), gerr) {
		t.Error("wrapped googleapi.Error not detected through errors.Is")
	}
}
