package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/vietdv277/cumulus/pkg/types"
)

// AWSK8sProvider implements provider.K8sProvider for Amazon EKS.
type AWSK8sProvider struct {
	client  *Client
	profile string
	region  string
}

// NewK8sProvider creates a new EKS-backed K8sProvider.
func NewK8sProvider(client *Client, profile, region string) *AWSK8sProvider {
	return &AWSK8sProvider{client: client, profile: profile, region: region}
}

// ListClusters returns all EKS clusters in the configured region.
// Node count is not populated here — call GetCluster for a detailed view.
func (p *AWSK8sProvider) ListClusters(ctx context.Context) ([]types.K8sCluster, error) {
	var names []string
	paginator := eks.NewListClustersPaginator(p.client.EKS(), &eks.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list EKS clusters: %w", err)
		}
		names = append(names, page.Clusters...)
	}

	if len(names) == 0 {
		return nil, nil
	}

	// Describe clusters in parallel for latency.
	clusters := make([]types.K8sCluster, len(names))
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			out, err := p.client.EKS().DescribeCluster(ctx, &eks.DescribeClusterInput{Name: strPtr(name)})
			if err != nil {
				errs[i] = fmt.Errorf("describe %s: %w", name, err)
				return
			}
			if out.Cluster != nil {
				clusters[i] = eksToCluster(*out.Cluster, p.region)
			}
		}(i, name)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return clusters, nil
}

// GetCluster returns cluster details including aggregate node count across nodegroups.
func (p *AWSK8sProvider) GetCluster(ctx context.Context, nameOrID string) (*types.K8sCluster, error) {
	out, err := p.client.EKS().DescribeCluster(ctx, &eks.DescribeClusterInput{Name: strPtr(nameOrID)})
	if err != nil {
		return nil, fmt.Errorf("cluster not found: %s", nameOrID)
	}
	if out.Cluster == nil {
		return nil, fmt.Errorf("cluster not found: %s", nameOrID)
	}

	cluster := eksToCluster(*out.Cluster, p.region)
	cluster.NodeCount = p.sumNodegroupDesiredSize(ctx, nameOrID)
	return &cluster, nil
}

// UpdateKubeconfig delegates to the AWS CLI, which already handles the
// IAM-authenticator exec plugin wiring in ~/.kube/config.
func (p *AWSK8sProvider) UpdateKubeconfig(ctx context.Context, nameOrID string) error {
	args := []string{"eks", "update-kubeconfig", "--name", nameOrID}
	if p.region != "" {
		args = append(args, "--region", p.region)
	}
	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws eks update-kubeconfig failed (is the AWS CLI installed?): %w", err)
	}
	return nil
}

// sumNodegroupDesiredSize returns the total desired node count across all
// managed nodegroups. Returns 0 on any error — node count is informational.
func (p *AWSK8sProvider) sumNodegroupDesiredSize(ctx context.Context, cluster string) int {
	var total int
	paginator := eks.NewListNodegroupsPaginator(p.client.EKS(), &eks.ListNodegroupsInput{
		ClusterName: strPtr(cluster),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return total
		}
		for _, ng := range page.Nodegroups {
			out, err := p.client.EKS().DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
				ClusterName:   strPtr(cluster),
				NodegroupName: strPtr(ng),
			})
			if err != nil || out.Nodegroup == nil || out.Nodegroup.ScalingConfig == nil {
				continue
			}
			if sz := out.Nodegroup.ScalingConfig.DesiredSize; sz != nil {
				total += int(*sz)
			}
		}
	}
	return total
}

func eksToCluster(c ekstypes.Cluster, region string) types.K8sCluster {
	cluster := types.K8sCluster{
		ID:       deref(c.Arn),
		Name:     deref(c.Name),
		Version:  deref(c.Version),
		Status:   string(c.Status),
		Endpoint: deref(c.Endpoint),
		Region:   region,
		Provider: "aws",
		Raw:      c,
	}
	if c.CreatedAt != nil {
		cluster.CreatedAt = *c.CreatedAt
	}
	return cluster
}
