package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// AWSVMProvider implements the VMProvider interface for AWS EC2
type AWSVMProvider struct {
	client  *Client
	profile string
	region  string
}

// NewVMProvider creates a new AWS VM provider
func NewVMProvider(client *Client, profile, region string) *AWSVMProvider {
	return &AWSVMProvider{
		client:  client,
		profile: profile,
		region:  region,
	}
}

// List returns EC2 instances matching the filter
func (p *AWSVMProvider) List(ctx context.Context, filter *provider.VMFilter) ([]types.VM, error) {
	// Build filters
	filters := []ec2types.Filter{}

	// State filter
	if filter != nil && filter.State != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("instance-state-name"),
			Values: []string{filter.State},
		})
	} else {
		// Default to running instances
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running"},
		})
	}

	// Name filter
	if filter != nil && filter.Name != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("tag:Name"),
			Values: []string{"*" + filter.Name + "*"},
		})
	}

	// Tag filters
	if filter != nil && filter.Tags != nil {
		for key, value := range filter.Tags {
			filters = append(filters, ec2types.Filter{
				Name:   aws.String("tag:" + key),
				Values: []string{value},
			})
		}
	}

	// Call AWS API
	input := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	var vms []types.VM
	paginator := ec2.NewDescribeInstancesPaginator(p.client.EC2(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe instances: %w", err)
		}
		for _, reservation := range page.Reservations {
			for _, inst := range reservation.Instances {
				vms = append(vms, ec2ToVM(inst))
			}
		}
	}

	return vms, nil
}

// Get returns a single VM by name or ID
func (p *AWSVMProvider) Get(ctx context.Context, nameOrID string) (*types.VM, error) {
	// Try by instance ID first
	if isInstanceID(nameOrID) {
		input := &ec2.DescribeInstancesInput{
			InstanceIds: []string{nameOrID},
		}

		output, err := p.client.EC2().DescribeInstances(ctx, input)
		if err != nil {
			if isNotFoundErr(err, "InvalidInstanceID.NotFound") {
				return nil, fmt.Errorf("instance not found: %s", nameOrID)
			}
			return nil, fmt.Errorf("failed to get instance %s: %w", nameOrID, err)
		}

		for _, reservation := range output.Reservations {
			if len(reservation.Instances) > 0 {
				vm := ec2ToVM(reservation.Instances[0])
				return &vm, nil
			}
		}
		return nil, fmt.Errorf("instance not found: %s", nameOrID)
	}

	// Search by name, filtering to non-terminated states so a terminated ASG
	// member with the same Name tag doesn't shadow a live instance.
	input := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{nameOrID},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"pending", "running", "stopping", "stopped"},
			},
		},
	}

	var matches []vmMatch
	paginator := ec2.NewDescribeInstancesPaginator(p.client.EC2(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to find instance %s: %w", nameOrID, err)
		}
		for _, reservation := range page.Reservations {
			for _, inst := range reservation.Instances {
				matches = append(matches, vmMatch{
					ID:    deref(inst.InstanceId),
					State: string(instanceStateName(inst)),
					VM:    ec2ToVM(inst),
				})
			}
		}
	}

	vm, err := selectInstance(matches, nameOrID)
	if err != nil {
		return nil, err
	}
	return vm, nil
}

// Start starts an EC2 instance
func (p *AWSVMProvider) Start(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2().StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	if err != nil {
		return fmt.Errorf("failed to start instance %s: %w", vm.ID, err)
	}
	return nil
}

// Stop stops an EC2 instance
func (p *AWSVMProvider) Stop(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2().StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	if err != nil {
		return fmt.Errorf("failed to stop instance %s: %w", vm.ID, err)
	}
	return nil
}

// Reboot reboots an EC2 instance
func (p *AWSVMProvider) Reboot(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2().RebootInstances(ctx, &ec2.RebootInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	if err != nil {
		return fmt.Errorf("failed to reboot instance %s: %w", vm.ID, err)
	}
	return nil
}

// Connect establishes an SSM session to the instance
func (p *AWSVMProvider) Connect(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	args := []string{"ssm", "start-session", "--target", vm.ID}

	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}

	if p.region != "" {
		args = append(args, "--region", p.region)
	}

	ssmCmd := exec.CommandContext(ctx, "aws", args...)
	ssmCmd.Stdin = os.Stdin
	ssmCmd.Stdout = os.Stdout
	ssmCmd.Stderr = os.Stderr

	return ssmCmd.Run()
}

// StartPortForward starts an SSM port-forwarding session and returns the running command.
// Caller owns the process lifecycle (Wait/Kill). Used by Tunnel (blocks on Wait) and by
// k8s connect (manages lifecycle around a subshell). Output is wired to os.Stderr so the
// caller's stdout stays clean for subshells / pipes. The session runs in its own process
// group (on unix) so teardown can signal the session-manager-plugin child too.
func (p *AWSVMProvider) StartPortForward(ctx context.Context, instanceID string, remotePort, localPort int) (*exec.Cmd, error) {
	cmd, err := p.portForwardCmd(ctx, instanceID, remotePort, localPort)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := startTunnelProc(cmd); err != nil {
		return nil, fmt.Errorf("failed to start SSM session: %w", err)
	}
	return cmd, nil
}

// portForwardCmd builds the `aws ssm start-session` port-forwarding command.
func (p *AWSVMProvider) portForwardCmd(ctx context.Context, instanceID string, remotePort, localPort int) (*exec.Cmd, error) {
	params := map[string][]string{
		"portNumber":      {strconv.Itoa(remotePort)},
		"localPortNumber": {strconv.Itoa(localPort)},
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", string(paramsJSON),
	}
	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}
	if p.region != "" {
		args = append(args, "--region", p.region)
	}

	return exec.CommandContext(ctx, "aws", args...), nil
}

// Tunnel creates a port forwarding tunnel via SSM (blocking).
func (p *AWSVMProvider) Tunnel(ctx context.Context, nameOrID string, opts *provider.TunnelOptions) error {
	if opts == nil {
		return fmt.Errorf("tunnel options required")
	}
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	cmd, err := p.portForwardCmd(ctx, vm.ID, opts.RemotePort, opts.LocalPort)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := startTunnelProc(cmd); err != nil {
		return fmt.Errorf("failed to start SSM session: %w", err)
	}

	waitDone := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		err := cmd.Wait()
		waitDone <- err
		close(exit)
	}()
	defer stopTunnel(cmd, exit)

	// The session runs in its own process group, so Ctrl-C reaches only cml;
	// without this handler cml would die on the default SIGINT action and
	// orphan the tunnel. Tear the group down on our own terms instead.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-waitDone:
		return err
	case <-sigCh:
		stopTunnel(cmd, exit)
		return nil
	}
}

// startTunnelProc starts a prepared tunnel command in its own process group
// (unix) so the session-manager-plugin child can be signalled with it.
func startTunnelProc(cmd *exec.Cmd) error {
	tunnelProcGroup(cmd)
	return cmd.Start()
}

// stopTunnel signals the tunnel's process group, escalating to a kill after a
// short grace period. exit must be closed by the Wait watcher.
func stopTunnel(cmd *exec.Cmd, exit <-chan struct{}) {
	if cmd.Process == nil {
		return
	}
	_ = TerminateTunnel(cmd)
	select {
	case <-exit:
	case <-time.After(3 * time.Second):
		_ = KillTunnel(cmd)
		<-exit
	}
}

// ec2ToVM converts an EC2 instance to the unified VM type
func ec2ToVM(i ec2types.Instance) types.VM {
	vm := types.VM{
		ID:       deref(i.InstanceId),
		State:    ec2StateToVMState(instanceStateName(i)),
		Type:     string(i.InstanceType),
		Provider: "aws",
		Tags:     make(map[string]string),
		Raw:      i,
	}

	if i.PrivateIpAddress != nil {
		vm.PrivateIP = *i.PrivateIpAddress
	}

	if i.PublicIpAddress != nil {
		vm.PublicIP = *i.PublicIpAddress
	}

	if i.Placement != nil && i.Placement.AvailabilityZone != nil {
		vm.Zone = *i.Placement.AvailabilityZone
	}

	if i.LaunchTime != nil {
		vm.LaunchedAt = *i.LaunchTime
	}

	// Extract tags
	for _, tag := range i.Tags {
		key := deref(tag.Key)
		value := deref(tag.Value)
		vm.Tags[key] = value

		switch key {
		case "Name":
			vm.Name = value
		case "aws:autoscaling:groupName":
			vm.ASG = value
		}
	}

	return vm
}

// instanceStateName safely reads an instance's state name; the SDK leaves
// State nil for some shapes.
func instanceStateName(i ec2types.Instance) ec2types.InstanceStateName {
	if i.State == nil {
		return ""
	}
	return i.State.Name
}

// isInstanceID reports whether the argument looks like an EC2 instance ID.
func isInstanceID(nameOrID string) bool {
	return strings.HasPrefix(nameOrID, "i-")
}

// vmMatch is one candidate instance found by a name lookup.
type vmMatch struct {
	ID    string
	State string
	VM    types.VM
}

// selectInstance picks the single candidate for a name lookup, rejecting
// ambiguous names with an error that lists every match.
func selectInstance(matches []vmMatch, name string) (*types.VM, error) {
	if len(matches) == 0 {
		return nil, fmt.Errorf("instance not found: %s", name)
	}
	if len(matches) == 1 {
		vm := matches[0].VM
		return &vm, nil
	}

	var b strings.Builder
	for _, m := range matches {
		b.WriteString("\n  " + m.ID + " (" + m.State + ")")
	}
	return nil, fmt.Errorf("multiple instances named %q; pass an instance ID instead:%s", name, b.String())
}

// ec2StateToVMState converts EC2 state to unified VMState
func ec2StateToVMState(state ec2types.InstanceStateName) types.VMState {
	switch state {
	case ec2types.InstanceStateNameRunning:
		return types.VMStateRunning
	case ec2types.InstanceStateNameStopped:
		return types.VMStateStopped
	case ec2types.InstanceStateNamePending:
		return types.VMStatePending
	case ec2types.InstanceStateNameStopping, ec2types.InstanceStateNameShuttingDown:
		return types.VMStateStopping
	default:
		return types.VMStateUnknown
	}
}
