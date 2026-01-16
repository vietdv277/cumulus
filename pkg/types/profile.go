package types

// AWSProfile represents an AWS CLI profile
type AWSProfile struct {
	Name   string
	Region string // from config file if set
	Source string // "credentials" or "config"
}
