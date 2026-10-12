// Package types holds shared domain types (VM, Secret, DB, Bucket, Cluster, …)
// used by the provider interfaces and their cloud-specific implementations.
package types

import "time"

// VMState represents the state of a VM
type VMState string

const (
	VMStateRunning  VMState = "running"
	VMStateStopped  VMState = "stopped"
	VMStatePending  VMState = "pending"
	VMStateStopping VMState = "stopping"
	VMStateUnknown  VMState = "unknown"
)

// VM represents a unified virtual machine model
type VM struct {
	ID         string            `json:"id" yaml:"id"`                   // Provider-specific ID
	Name       string            `json:"name" yaml:"name"`               // Name tag or instance name
	State      VMState           `json:"state" yaml:"state"`             // running, stopped, pending
	PrivateIP  string            `json:"private_ip" yaml:"private_ip"`   // Private IP address
	PublicIP   string            `json:"public_ip" yaml:"public_ip"`     // Public IP address (if any)
	Type       string            `json:"type" yaml:"type"`               // Instance type (t3.micro, e2-medium)
	Zone       string            `json:"zone" yaml:"zone"`               // Availability zone
	Tags       map[string]string `json:"tags" yaml:"tags"`               // All tags
	LaunchedAt time.Time         `json:"launched_at" yaml:"launched_at"` // Launch time
	Provider   string            `json:"provider" yaml:"provider"`       // aws, gcp

	// Provider-specific fields
	ASG string `json:"asg,omitempty" yaml:"asg,omitempty"` // AWS Auto Scaling Group

	// Raw holds the original API response for provider-specific access
	Raw interface{} `json:"-" yaml:"-"`
}

// IsRunning returns true if the VM is running
func (v *VM) IsRunning() bool {
	return v.State == VMStateRunning
}

// IsStopped returns true if the VM is stopped
func (v *VM) IsStopped() bool {
	return v.State == VMStateStopped
}

// GetTag returns a tag value by key
func (v *VM) GetTag(key string) string {
	if v.Tags == nil {
		return ""
	}
	return v.Tags[key]
}
