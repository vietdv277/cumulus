package types

import "time"

// Secret represents a secret metadata
type Secret struct {
	Name      string    `json:"name" yaml:"name"`             // Secret name or path
	ARN       string    `json:"arn" yaml:"arn"`               // Provider-specific identifier
	CreatedAt time.Time `json:"created_at" yaml:"created_at"` // Creation time
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"` // Last update time
	Provider  string    `json:"provider" yaml:"provider"`     // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-" yaml:"-"`
}

// SecretValue represents a secret with its value
type SecretValue struct {
	Secret
	Value   string `json:"value" yaml:"value"`     // The secret value
	Version string `json:"version" yaml:"version"` // Version identifier
}
