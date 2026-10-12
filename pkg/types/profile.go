package types

// AWSProfile represents an AWS CLI profile
type AWSProfile struct {
	Name   string `json:"name" yaml:"name"`
	Region string `json:"region,omitempty" yaml:"region,omitempty"` // from config file if set
	Source string `json:"source" yaml:"source"`                     // "credentials" or "config"
}
