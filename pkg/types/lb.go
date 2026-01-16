package types

import "time"

// LoadBalancer represents an AWS Load Balancer (ALB/NLB)
type LoadBalancer struct {
	Name      string
	ARN       string
	DNSName   string
	Type      string // application, network, gateway
	Scheme    string // internet-facing, internal
	State     string
	VPCID     string
	AZs       []string
	CreatedAt time.Time
}

// TargetGroup represents an AWS Target Group
type TargetGroup struct {
	Name     string
	ARN      string
	Protocol string
	Port     int
	VPCID    string
	Type     string // instance, ip, lambda
	LBARN    string // associated load balancer ARN
}

// Target represents a target in a target group
type Target struct {
	ID     string // instance ID or IP
	Port   int
	AZ     string
	Health string // healthy, unhealthy, draining, unused, initial
}

// Listener represents a load balancer listener
type Listener struct {
	ARN      string
	Port     int
	Protocol string
}
