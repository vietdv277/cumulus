package gcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	container "google.golang.org/api/container/v1"
	"google.golang.org/api/option"

	"github.com/vietdv277/cumulus/pkg/types"
)

// GCPK8sProvider implements provider.K8sProvider for Google Kubernetes Engine.
type GCPK8sProvider struct {
	client *Client
}

// NewK8sProvider creates a new GKE-backed K8sProvider.
func NewK8sProvider(client *Client) *GCPK8sProvider {
	return &GCPK8sProvider{client: client}
}

func (p *GCPK8sProvider) newService(ctx context.Context) (*container.Service, error) {
	return container.NewService(ctx,
		option.WithTokenSource(p.client.Credentials().TokenSource),
	)
}

// ListClusters returns all GKE clusters across all regions/zones in the project.
// GKE's Clusters.List with parent "projects/{project}/locations/-" aggregates
// both regional and zonal clusters in a single call.
func (p *GCPK8sProvider) ListClusters(ctx context.Context) ([]types.K8sCluster, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create container service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", p.client.Project())
	resp, err := svc.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list GKE clusters: %w", err)
	}

	region := p.client.Region()
	var clusters []types.K8sCluster
	for _, c := range resp.Clusters {
		if !locationMatchesRegion(c.Location, region) {
			continue
		}
		clusters = append(clusters, gkeToCluster(c))
	}
	return clusters, nil
}

// GetCluster finds a cluster by name and populates node count by summing
// across all node pools.
func (p *GCPK8sProvider) GetCluster(ctx context.Context, nameOrID string) (*types.K8sCluster, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create container service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", p.client.Project())
	resp, err := svc.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list GKE clusters: %w", err)
	}

	for _, c := range resp.Clusters {
		if c.Name != nameOrID {
			continue
		}
		cluster := gkeToCluster(c)
		for _, np := range c.NodePools {
			cluster.NodeCount += int(np.InitialNodeCount)
		}
		return &cluster, nil
	}
	return nil, fmt.Errorf("cluster not found: %s", nameOrID)
}

// UpdateKubeconfig delegates to gcloud, which installs gke-gcloud-auth-plugin
// configuration in ~/.kube/config automatically.
func (p *GCPK8sProvider) UpdateKubeconfig(ctx context.Context, nameOrID string) error {
	svc, err := p.newService(ctx)
	if err != nil {
		return fmt.Errorf("create container service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", p.client.Project())
	resp, err := svc.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list GKE clusters: %w", err)
	}

	var location string
	for _, c := range resp.Clusters {
		if c.Name == nameOrID {
			location = c.Location
			break
		}
	}
	if location == "" {
		return fmt.Errorf("cluster not found: %s", nameOrID)
	}

	locFlag := "--region"
	if isZone(location) {
		locFlag = "--zone"
	}

	cmd := exec.CommandContext(ctx, "gcloud", "container", "clusters", "get-credentials",
		nameOrID, locFlag, location, "--project", p.client.Project())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud container clusters get-credentials failed (is gcloud installed?): %w", err)
	}
	return nil
}

// locationMatchesRegion returns true if the cluster's location (zone or region)
// belongs to the configured region filter. Empty region matches everything.
func locationMatchesRegion(location, region string) bool {
	if region == "" {
		return true
	}
	if isZone(region) {
		return location == region
	}
	// region filter — match both the region itself and zones under it
	return location == region || strings.HasPrefix(location, region+"-")
}

func gkeToCluster(c *container.Cluster) types.K8sCluster {
	cluster := types.K8sCluster{
		ID:       c.SelfLink,
		Name:     c.Name,
		Version:  c.CurrentMasterVersion,
		Status:   c.Status,
		Endpoint: c.Endpoint,
		Region:   c.Location,
		Provider: "gcp",
		Raw:      c,
	}
	if c.CreateTime != "" {
		if t, err := time.Parse(time.RFC3339, c.CreateTime); err == nil {
			cluster.CreatedAt = t
		}
	}
	return cluster
}
