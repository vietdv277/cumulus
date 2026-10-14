package gcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	pam "cloud.google.com/go/privilegedaccessmanager/apiv1"
	"cloud.google.com/go/privilegedaccessmanager/apiv1/privilegedaccessmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// DefaultPAMLocation is where PAM entitlements live unless created regionally.
const DefaultPAMLocation = "global"

// GCPPAMProvider implements provider.PAMProvider for Privileged Access Manager.
type GCPPAMProvider struct {
	client   *Client
	location string
	svc      *pam.Client // reused when set, so tests can inject a fake endpoint
}

var _ provider.PAMProvider = (*GCPPAMProvider)(nil)

// NewPAMProvider creates a PAM provider for the client's project in location
// (empty means DefaultPAMLocation).
func NewPAMProvider(c *Client, location string) *GCPPAMProvider {
	if location == "" {
		location = DefaultPAMLocation
	}
	return &GCPPAMProvider{client: c, location: location}
}

func (p *GCPPAMProvider) newService(ctx context.Context) (*pam.Client, error) {
	if p.svc != nil {
		return p.svc, nil
	}
	svc, err := pam.NewRESTClient(ctx, option.WithTokenSource(p.client.Credentials().TokenSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create Privileged Access Manager client: %w", err)
	}
	p.svc = svc
	return p.svc, nil
}

func (p *GCPPAMProvider) parent() string {
	return fmt.Sprintf("projects/%s/locations/%s", p.client.Project(), p.location)
}

// ListEntitlements returns the entitlements the caller can request a grant on.
func (p *GCPPAMProvider) ListEntitlements(ctx context.Context) ([]types.Entitlement, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}
	it := svc.SearchEntitlements(ctx, &privilegedaccessmanagerpb.SearchEntitlementsRequest{
		Parent:           p.parent(),
		CallerAccessType: privilegedaccessmanagerpb.SearchEntitlementsRequest_GRANT_REQUESTER,
	})
	var out []types.Entitlement
	for {
		e, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to search entitlements: %w", err)
		}
		out = append(out, toEntitlement(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListGrants returns the grants the caller created on any entitlement in the
// location, newest first.
func (p *GCPPAMProvider) ListGrants(ctx context.Context) ([]types.Grant, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}
	it := svc.SearchGrants(ctx, &privilegedaccessmanagerpb.SearchGrantsRequest{
		Parent:             p.parent() + "/entitlements/-",
		CallerRelationship: privilegedaccessmanagerpb.SearchGrantsRequest_HAD_CREATED,
	})
	var out []types.Grant
	for {
		g, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to search grants: %w", err)
		}
		out = append(out, toGrant(g))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// GetGrant returns one grant by its full resource name.
func (p *GCPPAMProvider) GetGrant(ctx context.Context, name string) (*types.Grant, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}
	g, err := svc.GetGrant(ctx, &privilegedaccessmanagerpb.GetGrantRequest{Name: name})
	if err != nil {
		if isNotFound(err) {
			return nil, notFoundErr("grant", lastSegment(name))
		}
		return nil, fmt.Errorf("failed to get grant: %w", err)
	}
	grant := toGrant(g)
	return &grant, nil
}

// RequestGrant creates a grant on the entitlement (full resource name).
func (p *GCPPAMProvider) RequestGrant(ctx context.Context, entitlement string, req provider.GrantRequest) (*types.Grant, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}
	grant := &privilegedaccessmanagerpb.Grant{RequestedDuration: durationpb.New(req.Duration)}
	if req.Justification != "" {
		grant.Justification = &privilegedaccessmanagerpb.Justification{
			Justification: &privilegedaccessmanagerpb.Justification_UnstructuredJustification{
				UnstructuredJustification: req.Justification,
			},
		}
	}
	g, err := svc.CreateGrant(ctx, &privilegedaccessmanagerpb.CreateGrantRequest{Parent: entitlement, Grant: grant})
	if err != nil {
		if isNotFound(err) {
			return nil, notFoundErr("entitlement", lastSegment(entitlement))
		}
		return nil, fmt.Errorf("failed to request grant on %s: %w", lastSegment(entitlement), err)
	}
	out := toGrant(g)
	return &out, nil
}

func toEntitlement(e *privilegedaccessmanagerpb.Entitlement) types.Entitlement {
	resource, roles := iamAccess(e.GetPrivilegedAccess())
	out := types.Entitlement{
		ID:            lastSegment(e.GetName()),
		Name:          e.GetName(),
		Resource:      resource,
		Roles:         roles,
		MaxDuration:   e.GetMaxRequestDuration().AsDuration(),
		NeedsApproval: e.GetApprovalWorkflow().GetManualApprovals() != nil,
		State:         e.GetState().String(),
	}
	// An entitlement without a justification config is treated as optional.
	out.JustificationRequired = e.GetRequesterJustificationConfig().GetUnstructured() != nil
	return out
}

func toGrant(g *privilegedaccessmanagerpb.Grant) types.Grant {
	_, roles := iamAccess(g.GetPrivilegedAccess())
	out := types.Grant{
		ID:            lastSegment(g.GetName()),
		Name:          g.GetName(),
		Entitlement:   entitlementOf(g.GetName()),
		State:         g.GetState().String(),
		Roles:         roles,
		Duration:      g.GetRequestedDuration().AsDuration(),
		Justification: g.GetJustification().GetUnstructuredJustification(),
	}
	if t := g.GetCreateTime(); t != nil {
		out.CreatedAt = t.AsTime()
	}
	for _, ev := range g.GetTimeline().GetEvents() {
		if ev.GetActivated() != nil && ev.GetEventTime() != nil {
			out.ActivatedAt = ev.GetEventTime().AsTime()
			out.ExpiresAt = out.ActivatedAt.Add(out.Duration)
		}
	}
	return out
}

// iamAccess returns the resource and roles an entitlement or grant gives.
func iamAccess(pa *privilegedaccessmanagerpb.PrivilegedAccess) (string, []string) {
	iam := pa.GetGcpIamAccess()
	var roles []string
	for _, rb := range iam.GetRoleBindings() {
		roles = append(roles, rb.GetRole())
	}
	return iam.GetResource(), roles
}

// entitlementOf returns the entitlement ID in a grant's resource name
// (".../entitlements/<id>/grants/<grant>").
func entitlementOf(grantName string) string {
	parts := strings.Split(grantName, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "entitlements" {
			return parts[i+1]
		}
	}
	return ""
}

func lastSegment(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}
