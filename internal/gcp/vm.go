package gcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// GCPVMProvider implements provider.VMProvider for GCE instances.
type GCPVMProvider struct {
	client *Client
}

// NewVMProvider creates a new GCE VM provider backed by the given Client.
func NewVMProvider(client *Client) *GCPVMProvider {
	return &GCPVMProvider{client: client}
}

// isZone returns true when s looks like a GCE zone (e.g. "us-central1-a").
// A zone has at least two hyphens and ends with a letter a–f.
func isZone(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return strings.Count(s, "-") >= 2 && last >= 'a' && last <= 'f'
}

// newInstancesClient returns an authenticated GCE Instances REST client.
func (p *GCPVMProvider) newInstancesClient(ctx context.Context) (*compute.InstancesClient, error) {
	return compute.NewInstancesRESTClient(ctx,
		option.WithTokenSource(p.client.Credentials().TokenSource),
	)
}

// newInstanceGroupsClient returns an authenticated GCE InstanceGroups REST client.
func (p *GCPVMProvider) newInstanceGroupsClient(ctx context.Context) (*compute.InstanceGroupsClient, error) {
	return compute.NewInstanceGroupsRESTClient(ctx,
		option.WithTokenSource(p.client.Credentials().TokenSource),
	)
}

// enrichWithUMIG queries unmanaged instance groups and sets vm.ASG for any
// VM that is a member. Only called for VMs not already in a MIG.
func (p *GCPVMProvider) enrichWithUMIG(ctx context.Context, vms []types.VM) error {
	// Build selfLink → vm index map (only for VMs not already in a MIG)
	linkIndex := map[string]int{}
	for i, vm := range vms {
		if vm.ASG == "" {
			if raw, ok := vm.Raw.(*computepb.Instance); ok {
				linkIndex[raw.GetSelfLink()] = i
			}
		}
	}
	if len(linkIndex) == 0 {
		return nil // all VMs already have MIG info
	}

	igc, err := p.newInstanceGroupsClient(ctx)
	if err != nil {
		return fmt.Errorf("create instance groups client: %w", err)
	}
	defer func() { _ = igc.Close() }()

	region := p.client.Region()
	req := &computepb.AggregatedListInstanceGroupsRequest{
		Project: p.client.Project(),
	}
	it := igc.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("list instance groups: %w", err)
		}

		zoneName := strings.TrimPrefix(pair.Key, "zones/")
		// Apply same region filter as listAggregated
		if region != "" && !isZone(region) && !strings.HasPrefix(zoneName, region+"-") {
			continue
		}
		if isZone(region) && zoneName != region {
			continue
		}

		for _, ig := range pair.Value.GetInstanceGroups() {
			igName := ig.GetName()
			memberReq := &computepb.ListInstancesInstanceGroupsRequest{
				Project:       p.client.Project(),
				Zone:          zoneName,
				InstanceGroup: igName,
			}
			memberIt := igc.ListInstances(ctx, memberReq)
			for {
				member, err := memberIt.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					break // skip this group on error
				}
				if idx, ok := linkIndex[member.GetInstance()]; ok {
					vms[idx].ASG = igName
				}
			}
		}
	}
	return nil
}

// buildFilter converts a VMFilter into a GCE filter string.
func buildFilter(filter *provider.VMFilter) string {
	var parts []string

	if filter != nil {
		switch filter.State {
		case "running":
			parts = append(parts, "status=RUNNING")
		case "stopped":
			parts = append(parts, "status=TERMINATED")
		case "", "all":
			// no state filter
		default:
			parts = append(parts, fmt.Sprintf("status=%s", strings.ToUpper(filter.State)))
		}

		if filter.Name != "" {
			// GCE name filter uses RE2: prefix match
			parts = append(parts, fmt.Sprintf("name:%s", filter.Name))
		}

		for k, v := range filter.Tags {
			parts = append(parts, fmt.Sprintf("labels.%s=%s", k, v))
		}
	}

	return strings.Join(parts, " AND ")
}

// gceToVM converts a GCE Instance proto to the unified VM type.
func gceToVM(inst *computepb.Instance) types.VM {
	vm := types.VM{
		ID:       fmt.Sprintf("%d", inst.GetId()),
		Name:     inst.GetName(),
		State:    gceStatusToVMState(inst.GetStatus()),
		Type:     path.Base(inst.GetMachineType()),
		Zone:     path.Base(inst.GetZone()),
		Provider: "gcp",
		Tags:     inst.GetLabels(),
		Raw:      inst,
	}

	if vm.Tags == nil {
		vm.Tags = make(map[string]string)
	}

	// Network interfaces
	if nics := inst.GetNetworkInterfaces(); len(nics) > 0 {
		vm.PrivateIP = nics[0].GetNetworkIP()
		if acs := nics[0].GetAccessConfigs(); len(acs) > 0 {
			vm.PublicIP = acs[0].GetNatIP()
		}
	}

	// Launch time
	if ts := inst.GetCreationTimestamp(); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			vm.LaunchedAt = t
		}
	}

	// MIG membership: GCE sets "created-by" metadata automatically
	// e.g. "projects/123/zones/us-central1-a/instanceGroupManagers/my-mig"
	if meta := inst.GetMetadata(); meta != nil {
		for _, item := range meta.GetItems() {
			if item.GetKey() == "created-by" {
				vm.ASG = path.Base(item.GetValue())
				break
			}
		}
	}

	return vm
}

// gceStatusToVMState maps a GCE instance status string to VMState.
func gceStatusToVMState(status string) types.VMState {
	switch status {
	case "RUNNING":
		return types.VMStateRunning
	case "TERMINATED", "SUSPENDED":
		return types.VMStateStopped
	case "PROVISIONING", "STAGING":
		return types.VMStatePending
	case "STOPPING", "SUSPENDING":
		return types.VMStateStopping
	default:
		return types.VMStateUnknown
	}
}

// List returns GCE instances matching the filter.
// If the configured region looks like a zone it performs a zone-scoped List;
// otherwise it performs an AggregatedList and filters by region prefix.
func (p *GCPVMProvider) List(ctx context.Context, filter *provider.VMFilter) ([]types.VM, error) {
	ic, err := p.newInstancesClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create instances client: %w", err)
	}
	defer func() { _ = ic.Close() }()

	gceFilter := buildFilter(filter)
	region := p.client.Region()

	var vms []types.VM
	var listErr error
	if isZone(region) {
		vms, listErr = p.listByZone(ctx, ic, region, gceFilter)
	} else {
		vms, listErr = p.listAggregated(ctx, ic, region, gceFilter)
	}
	if listErr != nil {
		return nil, listErr
	}
	// UMIG enrichment (best-effort; MIG membership already set via metadata)
	_ = p.enrichWithUMIG(ctx, vms)
	return vms, nil
}

func (p *GCPVMProvider) listByZone(ctx context.Context, ic *compute.InstancesClient, zone, gceFilter string) ([]types.VM, error) {
	req := &computepb.ListInstancesRequest{
		Project: p.client.Project(),
		Zone:    zone,
	}
	if gceFilter != "" {
		req.Filter = &gceFilter
	}

	var vms []types.VM
	it := ic.List(ctx, req)
	for {
		inst, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list instances: %w", err)
		}
		vms = append(vms, gceToVM(inst))
	}
	return vms, nil
}

func (p *GCPVMProvider) listAggregated(ctx context.Context, ic *compute.InstancesClient, region, gceFilter string) ([]types.VM, error) {
	req := &computepb.AggregatedListInstancesRequest{
		Project: p.client.Project(),
	}
	if gceFilter != "" {
		req.Filter = &gceFilter
	}

	var vms []types.VM
	it := ic.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("aggregated list instances: %w", err)
		}

		// Filter to zones belonging to the requested region (or all if region is empty)
		zoneName := strings.TrimPrefix(pair.Key, "zones/")
		if region != "" && !strings.HasPrefix(zoneName, region+"-") {
			continue
		}

		for _, inst := range pair.Value.GetInstances() {
			vms = append(vms, gceToVM(inst))
		}
	}
	return vms, nil
}

// Get returns a single GCE instance by name or numeric ID.
// Because GCE's Get API requires the zone, we discover it via List.
func (p *GCPVMProvider) Get(ctx context.Context, nameOrID string) (*types.VM, error) {
	// List all states so we can find stopped instances too.
	all, err := p.List(ctx, &provider.VMFilter{State: "all"})
	if err != nil {
		return nil, err
	}

	for i := range all {
		if all[i].Name == nameOrID || all[i].ID == nameOrID {
			return &all[i], nil
		}
	}

	return nil, fmt.Errorf("instance not found: %s", nameOrID)
}

// resolveVM looks up a VM and returns it with the zone field populated.
func (p *GCPVMProvider) resolveVM(ctx context.Context, nameOrID string) (*types.VM, error) {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return nil, err
	}
	if vm.Zone == "" {
		return nil, fmt.Errorf("could not determine zone for instance %s", nameOrID)
	}
	return vm, nil
}

// Start starts a GCE instance and waits for the operation to complete.
func (p *GCPVMProvider) Start(ctx context.Context, nameOrID string) error {
	vm, err := p.resolveVM(ctx, nameOrID)
	if err != nil {
		return err
	}

	ic, err := p.newInstancesClient(ctx)
	if err != nil {
		return fmt.Errorf("create instances client: %w", err)
	}
	defer func() { _ = ic.Close() }()

	op, err := ic.Start(ctx, &computepb.StartInstanceRequest{
		Project:  p.client.Project(),
		Zone:     vm.Zone,
		Instance: vm.Name,
	})
	if err != nil {
		return fmt.Errorf("start instance: %w", err)
	}
	return op.Wait(ctx)
}

// Stop stops a GCE instance and waits for the operation to complete.
func (p *GCPVMProvider) Stop(ctx context.Context, nameOrID string) error {
	vm, err := p.resolveVM(ctx, nameOrID)
	if err != nil {
		return err
	}

	ic, err := p.newInstancesClient(ctx)
	if err != nil {
		return fmt.Errorf("create instances client: %w", err)
	}
	defer func() { _ = ic.Close() }()

	op, err := ic.Stop(ctx, &computepb.StopInstanceRequest{
		Project:  p.client.Project(),
		Zone:     vm.Zone,
		Instance: vm.Name,
	})
	if err != nil {
		return fmt.Errorf("stop instance: %w", err)
	}
	return op.Wait(ctx)
}

// Reboot power-cycles a GCE instance via instances.Reset and waits for completion.
func (p *GCPVMProvider) Reboot(ctx context.Context, nameOrID string) error {
	vm, err := p.resolveVM(ctx, nameOrID)
	if err != nil {
		return err
	}

	ic, err := p.newInstancesClient(ctx)
	if err != nil {
		return fmt.Errorf("create instances client: %w", err)
	}
	defer func() { _ = ic.Close() }()

	op, err := ic.Reset(ctx, &computepb.ResetInstanceRequest{
		Project:  p.client.Project(),
		Zone:     vm.Zone,
		Instance: vm.Name,
	})
	if err != nil {
		return fmt.Errorf("reset instance: %w", err)
	}
	return op.Wait(ctx)
}

// Connect opens an interactive SSH session via `gcloud compute ssh`.
func (p *GCPVMProvider) Connect(ctx context.Context, nameOrID string) error {
	vm, err := p.resolveVM(ctx, nameOrID)
	if err != nil {
		return err
	}

	if p.client.Bastion() != "" {
		bastionProject := p.client.BastionProject()
		if bastionProject == "" {
			bastionProject = p.client.Project()
		}
		bastionZone := p.client.BastionZone()
		if bastionZone == "" {
			bastionZone = p.client.Region()
		}
		args := []string{
			"compute",
			"--project", bastionProject,
			"ssh",
			"--zone", bastionZone,
			p.client.Bastion(),
			"--ssh-flag=-tA",
			"--command", fmt.Sprintf("ssh %s", vm.PrivateIP),
		}
		if p.client.BastionIAP() {
			args = append(args, "--tunnel-through-iap")
		}
		cmd := exec.CommandContext(ctx, "gcloud", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.CommandContext(ctx, "gcloud", "compute", "ssh", vm.Name,
		"--project", p.client.Project(),
		"--zone", vm.Zone,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Tunnel creates a port-forwarding tunnel via `gcloud compute ssh`.
func (p *GCPVMProvider) Tunnel(ctx context.Context, nameOrID string, opts *provider.TunnelOptions) error {
	if opts == nil {
		return fmt.Errorf("tunnel options required")
	}

	vm, err := p.resolveVM(ctx, nameOrID)
	if err != nil {
		return err
	}

	if p.client.Bastion() != "" {
		remoteHost := opts.RemoteHost
		if remoteHost == "" {
			remoteHost = vm.PrivateIP
		}
		localArg := fmt.Sprintf("%d:%s:%d", opts.LocalPort, remoteHost, opts.RemotePort)

		bastionProject := p.client.BastionProject()
		if bastionProject == "" {
			bastionProject = p.client.Project()
		}
		bastionZone := p.client.BastionZone()
		if bastionZone == "" {
			bastionZone = p.client.Region()
		}
		args := []string{
			"compute",
			"--project", bastionProject,
			"ssh",
			"--zone", bastionZone,
			p.client.Bastion(),
			"--ssh-flag=-A",
		}
		if p.client.BastionIAP() {
			args = append(args, "--tunnel-through-iap")
		}
		args = append(args, "--", "-N", "-L", localArg)
		cmd := exec.CommandContext(ctx, "gcloud", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	remoteHost := opts.RemoteHost
	if remoteHost == "" {
		remoteHost = "localhost"
	}

	localArg := fmt.Sprintf("%d:%s:%d", opts.LocalPort, remoteHost, opts.RemotePort)

	cmd := exec.CommandContext(ctx, "gcloud", "compute", "ssh", vm.Name,
		"--project", p.client.Project(),
		"--zone", vm.Zone,
		"--", "-N", "-L", localArg,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
