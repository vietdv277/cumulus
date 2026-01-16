package aws

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// ListASGInput contains parameters for listing Auto Scaling Groups
type ListASGInput struct {
	NamePattern string
	Names       []string
}

// ListAutoScalingGroups returns a list of Auto Scaling Groups
func (c *Client) ListAutoScalingGroups(input *ListASGInput) ([]pkgtypes.AutoScalingGroup, error) {
	if input == nil {
		input = &ListASGInput{}
	}

	var allGroups []asgtypes.AutoScalingGroup
	var nextToken *string

	for {
		describeInput := &autoscaling.DescribeAutoScalingGroupsInput{
			NextToken: nextToken,
		}

		if len(input.Names) > 0 {
			describeInput.AutoScalingGroupNames = input.Names
		}

		output, err := c.ASG.DescribeAutoScalingGroups(c.ctx, describeInput)
		if err != nil {
			return nil, fmt.Errorf("failed to describe auto scaling groups: %w", err)
		}

		allGroups = append(allGroups, output.AutoScalingGroups...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	// Convert to internal type and filter by name pattern
	var groups []pkgtypes.AutoScalingGroup
	for _, g := range allGroups {
		asg := toAutoScalingGroup(g)

		// Filter by name pattern if specified
		if input.NamePattern != "" {
			if !strings.Contains(strings.ToLower(asg.Name), strings.ToLower(input.NamePattern)) {
				continue
			}
		}

		groups = append(groups, asg)
	}

	return groups, nil
}

// DescribeAutoScalingGroup returns detailed info about a specific ASG including its instances
func (c *Client) DescribeAutoScalingGroup(name string) (*pkgtypes.AutoScalingGroup, error) {
	output, err := c.ASG.DescribeAutoScalingGroups(c.ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{name},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe auto scaling group: %w", err)
	}

	if len(output.AutoScalingGroups) == 0 {
		return nil, fmt.Errorf("auto scaling group %q not found", name)
	}

	asg := toAutoScalingGroup(output.AutoScalingGroups[0])

	// Get instance details
	if len(output.AutoScalingGroups[0].Instances) > 0 {
		var instanceIDs []string
		for _, inst := range output.AutoScalingGroups[0].Instances {
			if inst.InstanceId != nil {
				instanceIDs = append(instanceIDs, *inst.InstanceId)
			}
		}

		if len(instanceIDs) > 0 {
			instances, err := c.ListInstances(&ListInstanceInput{
				InstanceIDs: instanceIDs,
				States:      []string{"pending", "running", "stopping", "stopped"},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get ASG instances: %w", err)
			}
			asg.Instances = instances
		}
	}

	return &asg, nil
}

// UpdateASGInput contains parameters for updating an ASG
type UpdateASGInput struct {
	Name            string
	DesiredCapacity *int
	MinSize         *int
	MaxSize         *int
}

// UpdateAutoScalingGroup updates the capacity settings of an ASG
func (c *Client) UpdateAutoScalingGroup(input *UpdateASGInput) error {
	updateInput := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(input.Name),
	}

	if input.DesiredCapacity != nil {
		updateInput.DesiredCapacity = aws.Int32(int32(*input.DesiredCapacity))
	}
	if input.MinSize != nil {
		updateInput.MinSize = aws.Int32(int32(*input.MinSize))
	}
	if input.MaxSize != nil {
		updateInput.MaxSize = aws.Int32(int32(*input.MaxSize))
	}

	_, err := c.ASG.UpdateAutoScalingGroup(c.ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update auto scaling group: %w", err)
	}

	return nil
}

// RefreshInput contains parameters for starting an instance refresh
type RefreshInput struct {
	Name              string
	MinHealthyPercent int // default 90
}

// StartInstanceRefresh starts a rolling instance refresh
func (c *Client) StartInstanceRefresh(input *RefreshInput) (string, error) {
	minHealthy := input.MinHealthyPercent
	if minHealthy <= 0 {
		minHealthy = 90
	}

	refreshInput := &autoscaling.StartInstanceRefreshInput{
		AutoScalingGroupName: aws.String(input.Name),
		Strategy:             asgtypes.RefreshStrategyRolling,
		Preferences: &asgtypes.RefreshPreferences{
			MinHealthyPercentage: aws.Int32(int32(minHealthy)),
		},
	}

	output, err := c.ASG.StartInstanceRefresh(c.ctx, refreshInput)
	if err != nil {
		return "", fmt.Errorf("failed to start instance refresh: %w", err)
	}

	return deref(output.InstanceRefreshId), nil
}

// toAutoScalingGroup converts an AWS ASG type to our internal type
func toAutoScalingGroup(g asgtypes.AutoScalingGroup) pkgtypes.AutoScalingGroup {
	asg := pkgtypes.AutoScalingGroup{
		Name:            deref(g.AutoScalingGroupName),
		ARN:             deref(g.AutoScalingGroupARN),
		DesiredCapacity: int(deref32(g.DesiredCapacity)),
		MinSize:         int(deref32(g.MinSize)),
		MaxSize:         int(deref32(g.MaxSize)),
		Status:          deref(g.Status),
	}

	if g.CreatedTime != nil {
		asg.CreatedTime = *g.CreatedTime
	}

	// Get launch template name
	if g.LaunchTemplate != nil {
		asg.LaunchTemplate = deref(g.LaunchTemplate.LaunchTemplateName)
	} else if g.MixedInstancesPolicy != nil && g.MixedInstancesPolicy.LaunchTemplate != nil {
		if g.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification != nil {
			asg.LaunchTemplate = deref(g.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification.LaunchTemplateName)
		}
	}

	// Get availability zones
	asg.AZs = g.AvailabilityZones

	// Count instances by health status
	for _, inst := range g.Instances {
		asg.InstanceCount++
		if inst.HealthStatus != nil {
			if *inst.HealthStatus == "Healthy" {
				asg.HealthyCount++
			} else {
				asg.UnhealthyCount++
			}
		}
	}

	// Set status if not provided
	if asg.Status == "" {
		asg.Status = "InService"
	}

	return asg
}

// deref32 safely dereferences an int32 pointer
func deref32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}
