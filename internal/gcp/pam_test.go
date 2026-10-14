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

	pam "cloud.google.com/go/privilegedaccessmanager/apiv1"
	"google.golang.org/api/option"

	"github.com/vietdv277/cumulus/pkg/provider"
)

// newTestPAMProvider returns a provider whose PAM client talks to a test
// server running handler, recording every request.
func newTestPAMProvider(t *testing.T, handler http.HandlerFunc) (*GCPPAMProvider, *[]capturedRequest) {
	t.Helper()
	reqs := &[]capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*reqs = append(*reqs, capturedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := pam.NewRESTClient(context.Background(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("building test PAM client: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	p := NewPAMProvider(&Client{project: "proj"}, "")
	p.svc = svc
	return p, reqs
}

func TestGCPPAMListEntitlements(t *testing.T) {
	p, reqs := newTestPAMProvider(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entitlements":[
			{"name":"projects/proj/locations/global/entitlements/viewer",
			 "maxRequestDuration":"3600s","state":"AVAILABLE",
			 "requesterJustificationConfig":{"notMandatory":{}},
			 "privilegedAccess":{"gcpIamAccess":{"resourceType":"cloudresourcemanager.googleapis.com/Project",
			   "resource":"//cloudresourcemanager.googleapis.com/projects/proj",
			   "roleBindings":[{"role":"roles/viewer"}]}}},
			{"name":"projects/proj/locations/global/entitlements/admin",
			 "maxRequestDuration":"86400s","state":"AVAILABLE",
			 "requesterJustificationConfig":{"unstructured":{}},
			 "approvalWorkflow":{"manualApprovals":{"steps":[{"approvalsNeeded":1}]}},
			 "privilegedAccess":{"gcpIamAccess":{"roleBindings":[{"role":"roles/compute.admin"},{"role":"roles/storage.admin"}]}}}
		]}`)
	})

	got, err := p.ListEntitlements(context.Background())
	if err != nil {
		t.Fatalf("ListEntitlements() error = %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(*reqs))
	}
	req := (*reqs)[0]
	if req.method != http.MethodGet || req.path != "/v1/projects/proj/locations/global/entitlements:search" {
		t.Errorf("request = %s %s, want GET .../locations/global/entitlements:search", req.method, req.path)
	}
	if want := "callerAccessType=GRANT_REQUESTER"; !strings.Contains(req.query, want) {
		t.Errorf("query = %q, want it to contain %q", req.query, want)
	}

	if len(got) != 2 || got[0].ID != "admin" || got[1].ID != "viewer" {
		t.Fatalf("entitlements = %+v, want admin then viewer (sorted by ID)", got)
	}
	admin, viewer := got[0], got[1]
	if !admin.NeedsApproval || !admin.JustificationRequired || admin.MaxDuration != 24*time.Hour ||
		len(admin.Roles) != 2 || admin.Roles[0] != "roles/compute.admin" {
		t.Errorf("admin = %+v, want approval, justification, 24h, two roles", admin)
	}
	if viewer.NeedsApproval || viewer.JustificationRequired || viewer.MaxDuration != time.Hour ||
		viewer.Resource != "//cloudresourcemanager.googleapis.com/projects/proj" {
		t.Errorf("viewer = %+v, want no approval, optional justification, 1h, project resource", viewer)
	}
}

func TestGCPPAMListGrantsNewestFirst(t *testing.T) {
	p, reqs := newTestPAMProvider(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"grants":[
			{"name":"projects/proj/locations/global/entitlements/viewer/grants/old",
			 "createTime":"2024-01-01T00:00:00Z","state":"ENDED","requestedDuration":"3600s"},
			{"name":"projects/proj/locations/global/entitlements/admin/grants/new",
			 "createTime":"2024-01-02T00:00:00Z","state":"ACTIVE","requestedDuration":"7200s",
			 "justification":{"unstructuredJustification":"incident"},
			 "timeline":{"events":[
			   {"eventTime":"2024-01-02T00:00:01Z","requested":{}},
			   {"eventTime":"2024-01-02T00:00:05Z","activated":{}}]},
			 "privilegedAccess":{"gcpIamAccess":{"roleBindings":[{"role":"roles/compute.admin"}]}}}
		]}`)
	})

	got, err := p.ListGrants(context.Background())
	if err != nil {
		t.Fatalf("ListGrants() error = %v", err)
	}
	req := (*reqs)[0]
	if req.path != "/v1/projects/proj/locations/global/entitlements/-/grants:search" {
		t.Errorf("path = %q, want the entitlements/- wildcard search", req.path)
	}
	if want := "callerRelationship=HAD_CREATED"; !strings.Contains(req.query, want) {
		t.Errorf("query = %q, want it to contain %q", req.query, want)
	}
	if len(got) != 2 || got[0].ID != "new" || got[1].ID != "old" {
		t.Fatalf("grants = %+v, want new then old", got)
	}
	g := got[0]
	activated := time.Date(2024, 1, 2, 0, 0, 5, 0, time.UTC)
	if g.Entitlement != "admin" || g.State != "ACTIVE" || g.Justification != "incident" ||
		!g.ActivatedAt.Equal(activated) || !g.ExpiresAt.Equal(activated.Add(2*time.Hour)) {
		t.Errorf("grant = %+v, want admin/ACTIVE/incident, expiring 2h after activation", g)
	}
	if !got[1].ExpiresAt.IsZero() {
		t.Errorf("old grant ExpiresAt = %v, want zero without an activation event", got[1].ExpiresAt)
	}
}

func TestGCPPAMRequestGrant(t *testing.T) {
	p, reqs := newTestPAMProvider(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"projects/proj/locations/global/entitlements/admin/grants/g1",
			"state":"APPROVAL_AWAITED","requestedDuration":"5400s"}`)
	})

	ent := "projects/proj/locations/global/entitlements/admin"
	got, err := p.RequestGrant(context.Background(), ent,
		provider.GrantRequest{Duration: 90 * time.Minute, Justification: "deploy hotfix"})
	if err != nil {
		t.Fatalf("RequestGrant() error = %v", err)
	}
	req := (*reqs)[0]
	if req.method != http.MethodPost || req.path != "/v1/"+ent+"/grants" {
		t.Errorf("request = %s %s, want POST /v1/%s/grants", req.method, req.path, ent)
	}
	var body struct {
		RequestedDuration string `json:"requestedDuration"`
		Justification     struct {
			UnstructuredJustification string `json:"unstructuredJustification"`
		} `json:"justification"`
	}
	if err := json.Unmarshal([]byte(req.body), &body); err != nil {
		t.Fatalf("decoding body %q: %v", req.body, err)
	}
	if body.RequestedDuration != "5400s" || body.Justification.UnstructuredJustification != "deploy hotfix" {
		t.Errorf("body = %+v, want 5400s and the justification", body)
	}
	if got.ID != "g1" || got.Entitlement != "admin" || !got.Pending() {
		t.Errorf("grant = %+v, want g1 on admin, pending", got)
	}
}

func TestGCPPAMGetGrantNotFound(t *testing.T) {
	p, _ := newTestPAMProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found","status":"NOT_FOUND"}}`)
	})
	_, err := p.GetGrant(context.Background(), "projects/proj/locations/global/entitlements/admin/grants/nope")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("GetGrant() error = %v, want ErrNotFound", err)
	}
}

func TestGCPPAMRequestGrantKeepsAPIError(t *testing.T) {
	p, _ := newTestPAMProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"caller is not eligible","status":"PERMISSION_DENIED"}}`)
	})
	_, err := p.RequestGrant(context.Background(), "projects/proj/locations/global/entitlements/admin",
		provider.GrantRequest{Duration: time.Hour})
	if err == nil || !strings.Contains(err.Error(), "admin") || !strings.Contains(err.Error(), "not eligible") {
		t.Errorf("RequestGrant() error = %v, want the entitlement and the API message", err)
	}
	if isNotFound(err) || isTransient(err) {
		t.Errorf("403 classified as not-found or transient: %v", err)
	}
}
