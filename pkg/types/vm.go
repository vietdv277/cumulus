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
	ID         string            `json:"id"`          // Provider-specific ID
	Name       string            `json:"name"`        // Name tag or instance name
	State      VMState           `json:"state"`       // running, stopped, pending
	PrivateIP  string            `json:"private_ip"`  // Private IP address
	PublicIP   string            `json:"public_ip"`   // Public IP address (if any)
	Type       string            `json:"type"`        // Instance type (t3.micro, e2-medium)
	Zone       string            `json:"zone"`        // Availability zone
	Tags       map[string]string `json:"tags"`        // All tags
	LaunchedAt time.Time         `json:"launched_at"` // Launch time
	Provider   string            `json:"provider"`    // aws, gcp

	// Provider-specific fields
	ASG string `json:"asg,omitempty"` // AWS Auto Scaling Group

	// Raw holds the original API response for provider-specific access
	Raw interface{} `json:"-"`
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
