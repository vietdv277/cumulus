package types

import "time"

// Instance represents a compute instance (EC2 or GCE)
type Instance struct {
	ID         string
	Name       string
	PrivateIP  string
	PublicIP   string
	State      string
	Type       string
	AZ         string
	ASG        string
	LaunchTime time.Time
	Cloud      string // "aws" or "gcp"
}
