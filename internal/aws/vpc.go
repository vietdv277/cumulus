package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// ListVPCs returns all VPCs
func (c *Client) ListVPCs() ([]pkgtypes.VPC, error) {
	output, err := c.EC2.DescribeVpcs(c.ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}

	var vpcs []pkgtypes.VPC
	for _, v := range output.Vpcs {
		vpcs = append(vpcs, toVPC(v))
	}

	return vpcs, nil
}

// DescribeVPC returns detailed information about a specific VPC
func (c *Client) DescribeVPC(vpcID string) (*pkgtypes.VPC, error) {
	output, err := c.EC2.DescribeVpcs(c.ctx, &ec2.DescribeVpcsInput{
		VpcIds: []string{vpcID},
	})
	if err != nil {
		return nil, err
	}

	if len(output.Vpcs) == 0 {
		return nil, nil
	}

	vpc := toVPC(output.Vpcs[0])
	return &vpc, nil
}

// ListSubnets returns all subnets, optionally filtered by VPC ID
func (c *Client) ListSubnets(vpcID string) ([]pkgtypes.Subnet, error) {
	input := &ec2.DescribeSubnetsInput{}

	if vpcID != "" {
		input.Filters = []ec2types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		}
	}

	output, err := c.EC2.DescribeSubnets(c.ctx, input)
	if err != nil {
		return nil, err
	}

	var subnets []pkgtypes.Subnet
	for _, s := range output.Subnets {
		subnets = append(subnets, toSubnet(s))
	}

	return subnets, nil
}

// toVPC converts an EC2 VPC to our VPC type
func toVPC(v ec2types.Vpc) pkgtypes.VPC {
	vpc := pkgtypes.VPC{
		ID:        deref(v.VpcId),
		CIDR:      deref(v.CidrBlock),
		State:     string(v.State),
		IsDefault: derefBool(v.IsDefault),
		OwnerID:   deref(v.OwnerId),
	}

	// Extract Name tag
	for _, tag := range v.Tags {
		if deref(tag.Key) == "Name" {
			vpc.Name = deref(tag.Value)
			break
		}
	}

	return vpc
}

// toSubnet converts an EC2 Subnet to our Subnet type
func toSubnet(s ec2types.Subnet) pkgtypes.Subnet {
	subnet := pkgtypes.Subnet{
		ID:           deref(s.SubnetId),
		VPCID:        deref(s.VpcId),
		CIDR:         deref(s.CidrBlock),
		AZ:           deref(s.AvailabilityZone),
		AvailableIPs: int(derefInt32(s.AvailableIpAddressCount)),
		State:        string(s.State),
		Public:       derefBool(s.MapPublicIpOnLaunch),
	}

	// Extract Name tag
	for _, tag := range s.Tags {
		if deref(tag.Key) == "Name" {
			subnet.Name = deref(tag.Value)
			break
		}
	}

	return subnet
}

// derefBool safely dereferences a bool pointer
func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// derefInt32 safely dereferences an int32 pointer
func derefInt32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}
