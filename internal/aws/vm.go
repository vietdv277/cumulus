package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

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

	output, err := p.client.EC2.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	// Convert to unified VM type
	var vms []types.VM
	for _, reservation := range output.Reservations {
		for _, inst := range reservation.Instances {
			vms = append(vms, ec2ToVM(inst))
		}
	}

	return vms, nil
}

// Get returns a single VM by name or ID
func (p *AWSVMProvider) Get(ctx context.Context, nameOrID string) (*types.VM, error) {
	// Try by instance ID first
	if len(nameOrID) > 0 && nameOrID[0] == 'i' && nameOrID[1] == '-' {
		input := &ec2.DescribeInstancesInput{
			InstanceIds: []string{nameOrID},
		}

		output, err := p.client.EC2.DescribeInstances(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("instance not found: %s", nameOrID)
		}

		if len(output.Reservations) > 0 && len(output.Reservations[0].Instances) > 0 {
			vm := ec2ToVM(output.Reservations[0].Instances[0])
			return &vm, nil
		}
		return nil, fmt.Errorf("instance not found: %s", nameOrID)
	}

	// Search by name
	input := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{nameOrID},
			},
		},
	}

	output, err := p.client.EC2.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to find instance: %w", err)
	}

	if len(output.Reservations) == 0 || len(output.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance not found: %s", nameOrID)
	}

	vm := ec2ToVM(output.Reservations[0].Instances[0])
	return &vm, nil
}

// Start starts an EC2 instance
func (p *AWSVMProvider) Start(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	return err
}

// Stop stops an EC2 instance
func (p *AWSVMProvider) Stop(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	return err
}

// Reboot reboots an EC2 instance
func (p *AWSVMProvider) Reboot(ctx context.Context, nameOrID string) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	_, err = p.client.EC2.RebootInstances(ctx, &ec2.RebootInstancesInput{
		InstanceIds: []string{vm.ID},
	})
	return err
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

	ssmCmd := exec.Command("aws", args...)
	ssmCmd.Stdin = os.Stdin
	ssmCmd.Stdout = os.Stdout
	ssmCmd.Stderr = os.Stderr

	return ssmCmd.Run()
}

// Tunnel creates a port forwarding tunnel via SSM
func (p *AWSVMProvider) Tunnel(ctx context.Context, nameOrID string, opts *provider.TunnelOptions) error {
	vm, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	if opts == nil {
		return fmt.Errorf("tunnel options required")
	}

	// Build the parameters JSON
	params := map[string][]string{
		"portNumber":      {strconv.Itoa(opts.RemotePort)},
		"localPortNumber": {strconv.Itoa(opts.LocalPort)},
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	args := []string{
		"ssm", "start-session",
		"--target", vm.ID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", string(paramsJSON),
	}

	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}

	if p.region != "" {
		args = append(args, "--region", p.region)
	}

	ssmCmd := exec.Command("aws", args...)
	ssmCmd.Stdin = os.Stdin
	ssmCmd.Stdout = os.Stdout
	ssmCmd.Stderr = os.Stderr

	return ssmCmd.Run()
}

// ec2ToVM converts an EC2 instance to the unified VM type
func ec2ToVM(i ec2types.Instance) types.VM {
	vm := types.VM{
		ID:       deref(i.InstanceId),
		State:    ec2StateToVMState(i.State.Name),
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
