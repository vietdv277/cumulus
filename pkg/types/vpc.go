package types

// VPC represents an AWS VPC
type VPC struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	CIDR      string `json:"cidr" yaml:"cidr"`
	State     string `json:"state" yaml:"state"`
	IsDefault bool   `json:"is_default" yaml:"is_default"`
	OwnerID   string `json:"owner_id" yaml:"owner_id"`
}

// Subnet represents an AWS VPC Subnet
type Subnet struct {
	ID           string `json:"id" yaml:"id"`
	Name         string `json:"name" yaml:"name"`
	VPCID        string `json:"vpc_id" yaml:"vpc_id"`
	CIDR         string `json:"cidr" yaml:"cidr"`
	AZ           string `json:"az" yaml:"az"`
	AvailableIPs int    `json:"available_ips" yaml:"available_ips"`
	State        string `json:"state" yaml:"state"`
	Public       bool   `json:"public" yaml:"public"` // MapPublicIpOnLaunch
}
