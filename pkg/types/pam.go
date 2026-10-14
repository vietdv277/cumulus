package types

import "time"

// Entitlement is a Privileged Access Manager entitlement the caller can
// request a grant against.
type Entitlement struct {
	ID                    string        `json:"id" yaml:"id"`
	Name                  string        `json:"name" yaml:"name"` // full resource name
	Resource              string        `json:"resource" yaml:"resource"`
	Roles                 []string      `json:"roles" yaml:"roles"`
	MaxDuration           time.Duration `json:"max_duration" yaml:"max_duration"`
	NeedsApproval         bool          `json:"needs_approval" yaml:"needs_approval"`
	JustificationRequired bool          `json:"justification_required" yaml:"justification_required"`
	State                 string        `json:"state" yaml:"state"`
}

// Grant is a request for temporary access through an entitlement.
type Grant struct {
	ID            string        `json:"id" yaml:"id"`
	Name          string        `json:"name" yaml:"name"` // full resource name
	Entitlement   string        `json:"entitlement" yaml:"entitlement"`
	State         string        `json:"state" yaml:"state"`
	Roles         []string      `json:"roles" yaml:"roles"`
	Duration      time.Duration `json:"duration" yaml:"duration"`
	Justification string        `json:"justification,omitempty" yaml:"justification,omitempty"`
	CreatedAt     time.Time     `json:"created_at" yaml:"created_at"`
	ActivatedAt   time.Time     `json:"activated_at,omitempty" yaml:"activated_at,omitempty"`
	// ExpiresAt is when an activated grant's access ends (activation + duration).
	ExpiresAt time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// Open reports whether the grant gives, or may still give, access.
func (g Grant) Open() bool {
	switch g.State {
	case "APPROVAL_AWAITED", "SCHEDULED", "ACTIVATING", "ACTIVE":
		return true
	}
	return false
}

// Pending reports whether the grant is open but not active yet.
func (g Grant) Pending() bool {
	return g.Open() && g.State != "ACTIVE"
}
