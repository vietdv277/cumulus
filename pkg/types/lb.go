package types

import "time"

// LoadBalancer represents an AWS Load Balancer (ALB/NLB)
type LoadBalancer struct {
	Name      string    `json:"name" yaml:"name"`
	ARN       string    `json:"arn" yaml:"arn"`
	DNSName   string    `json:"dns_name" yaml:"dns_name"`
	Type      string    `json:"type" yaml:"type"`     // application, network, gateway
	Scheme    string    `json:"scheme" yaml:"scheme"` // internet-facing, internal
	State     string    `json:"state" yaml:"state"`
	VPCID     string    `json:"vpc_id" yaml:"vpc_id"`
	AZs       []string  `json:"azs" yaml:"azs"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// TargetGroup represents an AWS Target Group
type TargetGroup struct {
	Name     string `json:"name" yaml:"name"`
	ARN      string `json:"arn" yaml:"arn"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Port     int    `json:"port" yaml:"port"`
	VPCID    string `json:"vpc_id" yaml:"vpc_id"`
	Type     string `json:"type" yaml:"type"`                         // instance, ip, lambda
	LBARN    string `json:"lb_arn,omitempty" yaml:"lb_arn,omitempty"` // associated load balancer ARN
}

// Target represents a target in a target group
type Target struct {
	ID     string `json:"id" yaml:"id"` // instance ID or IP
	Port   int    `json:"port" yaml:"port"`
	AZ     string `json:"az,omitempty" yaml:"az,omitempty"`
	Health string `json:"health" yaml:"health"` // healthy, unhealthy, draining, unused, initial
}

// Listener represents a load balancer listener
type Listener struct {
	ARN      string `json:"arn" yaml:"arn"`
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
}
