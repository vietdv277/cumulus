package types

import "time"

// Secret represents a secret metadata
type Secret struct {
	Name      string    `json:"name"`       // Secret name or path
	ARN       string    `json:"arn"`        // Provider-specific identifier
	CreatedAt time.Time `json:"created_at"` // Creation time
	UpdatedAt time.Time `json:"updated_at"` // Last update time
	Provider  string    `json:"provider"`   // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-"`
}

// SecretValue represents a secret with its value
type SecretValue struct {
	Secret
	Value   string `json:"value"`   // The secret value
	Version string `json:"version"` // Version identifier
}
