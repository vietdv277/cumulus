package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

const testEntitlementName = "projects/my-project/locations/global/entitlements/"

// fakePAM is an in-memory PAMProvider; GetGrant walks states in order.
type fakePAM struct {
	ents      []types.Entitlement
	grants    []types.Grant
	states    []string // states[0] from RequestGrant, then one per GetGrant
	requested *provider.GrantRequest
	gets      int
}

func (f *fakePAM) ListEntitlements(context.Context) ([]types.Entitlement, error) { return f.ents, nil }
func (f *fakePAM) ListGrants(context.Context) ([]types.Grant, error)             { return f.grants, nil }

func (f *fakePAM) GetGrant(_ context.Context, name string) (*types.Grant, error) {
	state := f.states[min(f.gets+1, len(f.states)-1)]
	f.gets++
	return &types.Grant{ID: "g1", Name: name, Entitlement: "admin", State: state}, nil
}

func (f *fakePAM) RequestGrant(_ context.Context, ent string, req provider.GrantRequest) (*types.Grant, error) {
	f.requested = &req
	state := "ACTIVE"
	if len(f.states) > 0 {
		state = f.states[0]
	}
	return &types.Grant{ID: "g1", Name: ent + "/grants/g1", Entitlement: "admin", State: state, Duration: req.Duration}, nil
}

func testEntitlements() []types.Entitlement {
	return []types.Entitlement{
		{ID: "admin", Name: testEntitlementName + "admin", MaxDuration: 4 * time.Hour, JustificationRequired: true, NeedsApproval: true},
		{ID: "viewer", Name: testEntitlementName + "viewer", MaxDuration: time.Hour},
	}
}

func newTestRequester(f *fakePAM, stdin string) (*pamRequester, *bytes.Buffer, *int) {
	status := &bytes.Buffer{}
	sleeps := new(int)
	r := &pamRequester{
		p:      f,
		in:     strings.NewReader(stdin),
		status: status,
		sleep: func(context.Context, time.Duration) error {
			*sleeps++
			return nil
		},
		interval: time.Second,
	}
	return r, status, sleeps
}

func setStdinTerminal(t *testing.T, tty bool) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return tty }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

func TestPAMRequestCreatesGrant(t *testing.T) {
	setStdinTerminal(t, false)
	f := &fakePAM{ents: testEntitlements()}
	r, _, _ := newTestRequester(f, "")

	g, err := r.request(context.Background(), "admin", 2*time.Hour, "  INC-1 hotfix ", false)
	if err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if f.requested == nil || f.requested.Duration != 2*time.Hour || f.requested.Justification != "INC-1 hotfix" {
		t.Errorf("requested = %+v, want 2h and the trimmed justification", f.requested)
	}
	if g.ID != "g1" {
		t.Errorf("grant = %+v, want g1", g)
	}
}

func TestPAMRequestValidation(t *testing.T) {
	setStdinTerminal(t, false)
	tests := []struct {
		name, id, just string
		d              time.Duration
		want           string
	}{
		{"unknown entitlement suggests", "admn", "x", time.Hour, `did you mean "admin"`},
		{"over the maximum", "viewer", "", 2 * time.Hour, "exceeds the 1h maximum"},
		{"non-positive duration", "viewer", "", 0, "must be positive"},
		{"missing justification without a terminal", "admin", "", time.Hour, "requires a justification; pass -j"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakePAM{ents: testEntitlements()}
			r, _, _ := newTestRequester(f, "")
			_, err := r.request(context.Background(), tt.id, tt.d, tt.just, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			if f.requested != nil {
				t.Errorf("a grant was requested despite the error")
			}
		})
	}
}

func TestPAMRequestPromptsForJustification(t *testing.T) {
	setStdinTerminal(t, true)
	f := &fakePAM{ents: testEntitlements()}
	r, status, _ := newTestRequester(f, "rotate keys\n")

	if _, err := r.request(context.Background(), "admin", time.Hour, "", false); err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if !strings.Contains(status.String(), "Justification:") {
		t.Errorf("status = %q, want the prompt", status.String())
	}
	if f.requested == nil || f.requested.Justification != "rotate keys" {
		t.Errorf("requested = %+v, want the prompted justification", f.requested)
	}
}

func TestPAMRequestReusesOpenGrant(t *testing.T) {
	f := &fakePAM{
		ents: testEntitlements(),
		grants: []types.Grant{
			{ID: "old", Entitlement: "admin", State: "ENDED"},
			{ID: "live", Entitlement: "admin", State: "ACTIVE"},
		},
	}
	r, status, _ := newTestRequester(f, "")

	g, err := r.request(context.Background(), "admin", time.Hour, "x", false)
	if err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if f.requested != nil {
		t.Errorf("a new grant was requested despite an active one")
	}
	if g.ID != "live" || !strings.Contains(status.String(), "already exists") {
		t.Errorf("grant = %s, status = %q, want the active grant reported", g.ID, status.String())
	}
}

func TestPAMRequestWaitUntilActive(t *testing.T) {
	f := &fakePAM{ents: testEntitlements(), states: []string{"APPROVAL_AWAITED", "APPROVAL_AWAITED", "ACTIVATING", "ACTIVE"}}
	r, status, sleeps := newTestRequester(f, "")

	g, err := r.request(context.Background(), "admin", time.Hour, "x", true)
	if err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if g.State != "ACTIVE" || *sleeps != 3 {
		t.Errorf("state = %s after %d polls, want ACTIVE after 3", g.State, *sleeps)
	}
	// Only state changes are reported, not every poll.
	if n := strings.Count(status.String(), "Grant g1:"); n != 2 {
		t.Errorf("status = %q, want 2 transition lines (ACTIVATING, ACTIVE)", status.String())
	}
}

func TestPAMRequestWaitFailsOnDenied(t *testing.T) {
	f := &fakePAM{ents: testEntitlements(), states: []string{"APPROVAL_AWAITED", "DENIED"}}
	r, _, _ := newTestRequester(f, "")

	_, err := r.request(context.Background(), "admin", time.Hour, "x", true)
	if err == nil || !strings.Contains(err.Error(), "DENIED") {
		t.Errorf("error = %v, want the denial", err)
	}
}

func TestPAMRequestWaitStopsOnCancel(t *testing.T) {
	f := &fakePAM{ents: testEntitlements(), states: []string{"APPROVAL_AWAITED"}}
	r, status, _ := newTestRequester(f, "")
	r.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	g, err := r.request(context.Background(), "admin", time.Hour, "x", true)
	if err != nil {
		t.Fatalf("request() error = %v, want nil when the wait is interrupted", err)
	}
	if g.State != "APPROVAL_AWAITED" || !strings.Contains(status.String(), "Stopped waiting") {
		t.Errorf("state = %s, status = %q, want the pending grant and a note", g.State, status.String())
	}
}

func TestRolesSummary(t *testing.T) {
	tests := map[string][]string{
		"-":                  nil,
		"roles/viewer":       {"roles/viewer"},
		"custom/deployer +1": {"organizations/123/roles/deployer", "roles/viewer"},
		"custom/reader":      {"projects/p/roles/reader"},
	}
	for want, roles := range tests {
		if got := rolesSummary(roles); got != want {
			t.Errorf("rolesSummary(%v) = %q, want %q", roles, got, want)
		}
	}
}

func TestShortDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Hour:        "1h",
		90 * time.Minute: "1h30m",
		45 * time.Minute: "45m",
		24 * time.Hour:   "24h",
		90 * time.Second: "1m30s",
	} {
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestPAMRejectsAWSContext(t *testing.T) {
	setTestConfig(t, testConfigYAML)
	t.Cleanup(func() { pamContextFlag = "" })
	pamContextFlag = "aws:prod"
	_, err := getPAMProvider(context.Background())
	if err == nil || !strings.Contains(err.Error(), "needs a GCP context") {
		t.Errorf("error = %v, want the GCP-only refusal", err)
	}
}

func TestPAMGCPContextRoutesToProvider(t *testing.T) {
	setTestConfig(t, testConfigYAML)
	orig := newGCPPAMProvider
	t.Cleanup(func() { newGCPPAMProvider = orig; pamContextFlag = "" })

	var gotProject, gotLocation string
	newGCPPAMProvider = func(_ context.Context, c *config.Context, location string) (provider.PAMProvider, error) {
		gotProject, gotLocation = c.Project, location
		return &fakePAM{}, nil
	}
	pamContextFlag = "stg"
	if _, err := getPAMProvider(context.Background()); err != nil {
		t.Fatalf("getPAMProvider() error = %v", err)
	}
	if gotProject != "my-project" || gotLocation != "global" {
		t.Errorf("project/location = %q/%q, want my-project/global", gotProject, gotLocation)
	}
}
