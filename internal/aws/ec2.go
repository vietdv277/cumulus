package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/vietdinh/cumulus/pkg/types"
)

// ListInstanceInput contains parameters for listing EC2 instances
type ListInstanceInput struct {
	NamePattern string
	ASGName     string
	InstanceIDs []string
	States      []string
}

func (c *Client) ListInstances(input *ListInstanceInput) ([]*Instance, error) {
	if input == nil {
		input = &ListInstanceInput{}
	}

	// Default to running instances
	states := input.States
	if len(states) == 0 {
		states = []string{"running"}
	}

	// Build filters
	filters := []ec2types.Filter{
		{
			Name:   aws.String("instance-state-name"),
			Values: states,
		},
	}

	if input.NamePattern != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("tag:name"),
			Values: []string{"*" + input.NamePattern + "*"},
		})
	}

	if input.ASGName != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("tag:aws:autoscaling:groupName"),
			Values: []string{input.ASGName},
		})
	}

	// Build AWS API
	describeInput := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	if len(input.InstanceIDs) > 0 {
		describeInput.InstanceIds = input.InstanceIDs
	}

	// Call AWS API
	output, err := c.EC2.DescribeInstances(c.ctx, describeInput)
	if err != nil {
		return nil, err
	}

	// Convert to internal Instance type
	var instances []types.Instance
	for _, reservation := range output.Reservations {
		for _, inst := range reservation.Instances {
			instances = append(instances, toInstance(inst))
		}
	}

	return instances, nil
}

// toInstance converts an EC2 Instance to our Instance type
func toInstance(i ec2types.Instance) types.Instance {
	inst := types.Instance{
		ID:    deref(i.InstanceId),
		State: string(i.State.Name),
		Type:  string(i.InstanceType),
		Cloud: "aws",
	}

	if i.PrivateIpAddress != nil {
		inst.PrivateIP = *i.PrivateIpAddress
	}

	if i.PublicIpAddress != nil {
		inst.PublicIP = *i.PublicIpAddress
	}

	if i.Placement != nil && i.Placement.AvailabilityZone != nil {
		inst.AZ = *i.Placement.AvailabilityZone
	}

	if i.LaunchTime != nil {
		inst.LaunchTime = *i.LaunchTime
	}

	// Extract tags
	for _, tag := range i.Tags {
		key := deref(tag.Key)
		value := deref(tag.Value)

		switch key {
		case "Name":
			inst.Name = value
		case "aws:autoscaling:groupName":
			inst.ASG = value
		}
	}

	return inst
}

// deref safely dereferences a string pointer
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
