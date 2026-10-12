package aws

import (
	"fmt"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	pkgtypes "github.com/vietdv277/cumulus/pkg/types"
)

// ListLoadBalancers returns all load balancers (ALB/NLB)
func (c *Client) ListLoadBalancers() ([]pkgtypes.LoadBalancer, error) {
	var lbs []pkgtypes.LoadBalancer
	paginator := elbv2.NewDescribeLoadBalancersPaginator(c.ELBv2(), &elbv2.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe load balancers: %w", err)
		}
		for _, lb := range page.LoadBalancers {
			lbs = append(lbs, toLoadBalancer(lb))
		}
	}

	return lbs, nil
}

// DescribeLoadBalancer returns detailed information about a specific load balancer
func (c *Client) DescribeLoadBalancer(name string) (*pkgtypes.LoadBalancer, error) {
	output, err := c.ELBv2().DescribeLoadBalancers(c.ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{name},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe load balancer %s: %w", name, err)
	}

	if len(output.LoadBalancers) == 0 {
		return nil, nil
	}

	lb := toLoadBalancer(output.LoadBalancers[0])
	return &lb, nil
}

// GetLoadBalancerByName returns a load balancer by name
func (c *Client) GetLoadBalancerByName(name string) (*pkgtypes.LoadBalancer, error) {
	return c.DescribeLoadBalancer(name)
}

// ListListeners returns all listeners for a load balancer
func (c *Client) ListListeners(lbARN string) ([]pkgtypes.Listener, error) {
	var listeners []pkgtypes.Listener
	paginator := elbv2.NewDescribeListenersPaginator(c.ELBv2(), &elbv2.DescribeListenersInput{
		LoadBalancerArn: &lbARN,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe listeners: %w", err)
		}
		for _, l := range page.Listeners {
			listeners = append(listeners, toListener(l))
		}
	}

	return listeners, nil
}

// ListTargetGroups returns all target groups, optionally filtered by load balancer ARN
func (c *Client) ListTargetGroups(lbARN string) ([]pkgtypes.TargetGroup, error) {
	input := &elbv2.DescribeTargetGroupsInput{}
	if lbARN != "" {
		input.LoadBalancerArn = &lbARN
	}

	var tgs []pkgtypes.TargetGroup
	paginator := elbv2.NewDescribeTargetGroupsPaginator(c.ELBv2(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe target groups: %w", err)
		}
		for _, tg := range page.TargetGroups {
			tgs = append(tgs, toTargetGroup(tg, lbARN))
		}
	}

	return tgs, nil
}

// ListTargets returns all targets in a target group with their health status
func (c *Client) ListTargets(tgARN string) ([]pkgtypes.Target, error) {
	output, err := c.ELBv2().DescribeTargetHealth(c.ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: &tgARN,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe target health: %w", err)
	}

	var targets []pkgtypes.Target
	for _, thd := range output.TargetHealthDescriptions {
		targets = append(targets, toTarget(thd))
	}

	return targets, nil
}

// toLoadBalancer converts an ELBv2 LoadBalancer to our LoadBalancer type
func toLoadBalancer(lb elbv2types.LoadBalancer) pkgtypes.LoadBalancer {
	result := pkgtypes.LoadBalancer{
		Name:    deref(lb.LoadBalancerName),
		ARN:     deref(lb.LoadBalancerArn),
		DNSName: deref(lb.DNSName),
		Type:    string(lb.Type),
		Scheme:  string(lb.Scheme),
		VPCID:   deref(lb.VpcId),
	}

	if lb.State != nil {
		result.State = string(lb.State.Code)
	}

	if lb.CreatedTime != nil {
		result.CreatedAt = *lb.CreatedTime
	}

	for _, az := range lb.AvailabilityZones {
		if az.ZoneName != nil {
			result.AZs = append(result.AZs, *az.ZoneName)
		}
	}

	return result
}

// toListener converts an ELBv2 Listener to our Listener type
func toListener(l elbv2types.Listener) pkgtypes.Listener {
	return pkgtypes.Listener{
		ARN:      deref(l.ListenerArn),
		Port:     int(derefInt32(l.Port)),
		Protocol: string(l.Protocol),
	}
}

// toTargetGroup converts an ELBv2 TargetGroup to our TargetGroup type
func toTargetGroup(tg elbv2types.TargetGroup, lbARN string) pkgtypes.TargetGroup {
	result := pkgtypes.TargetGroup{
		Name:     deref(tg.TargetGroupName),
		ARN:      deref(tg.TargetGroupArn),
		Protocol: string(tg.Protocol),
		Port:     int(derefInt32(tg.Port)),
		VPCID:    deref(tg.VpcId),
		Type:     string(tg.TargetType),
		LBARN:    lbARN,
	}

	// If LB ARN not provided, try to get from LoadBalancerArns
	if result.LBARN == "" && len(tg.LoadBalancerArns) > 0 {
		result.LBARN = tg.LoadBalancerArns[0]
	}

	return result
}

// toTarget converts an ELBv2 TargetHealthDescription to our Target type
func toTarget(thd elbv2types.TargetHealthDescription) pkgtypes.Target {
	target := pkgtypes.Target{}

	if thd.Target != nil {
		target.ID = deref(thd.Target.Id)
		if thd.Target.Port != nil {
			target.Port = int(*thd.Target.Port)
		}
		target.AZ = deref(thd.Target.AvailabilityZone)
	}

	if thd.TargetHealth != nil {
		target.Health = string(thd.TargetHealth.State)
	}

	return target
}
