package types

import "time"

// Instance represents a compute instance (EC2 or GCE)
type Instance struct {
	ID         string    `json:"id" yaml:"id"`
	Name       string    `json:"name" yaml:"name"`
	PrivateIP  string    `json:"private_ip" yaml:"private_ip"`
	PublicIP   string    `json:"public_ip,omitempty" yaml:"public_ip,omitempty"`
	State      string    `json:"state" yaml:"state"`
	Type       string    `json:"type" yaml:"type"`
	AZ         string    `json:"az" yaml:"az"`
	ASG        string    `json:"asg,omitempty" yaml:"asg,omitempty"`
	LaunchTime time.Time `json:"launch_time" yaml:"launch_time"`
	Cloud      string    `json:"cloud" yaml:"cloud"` // "aws" or "gcp"
}
