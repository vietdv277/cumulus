package types

// VPC represents an AWS VPC
type VPC struct {
	ID        string
	Name      string
	CIDR      string
	State     string
	IsDefault bool
	OwnerID   string
}

// Subnet represents an AWS VPC Subnet
type Subnet struct {
	ID           string
	Name         string
	VPCID        string
	CIDR         string
	AZ           string
	AvailableIPs int
	State        string
	Public       bool // MapPublicIpOnLaunch
}
