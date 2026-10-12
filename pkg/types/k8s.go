package types

import "time"

// K8sCluster represents a Kubernetes cluster
type K8sCluster struct {
	ID        string    `json:"id" yaml:"id"`
	Name      string    `json:"name" yaml:"name"`
	Version   string    `json:"version" yaml:"version"`   // Kubernetes version
	Status    string    `json:"status" yaml:"status"`     // ACTIVE, CREATING, etc.
	Endpoint  string    `json:"endpoint" yaml:"endpoint"` // API server endpoint
	Region    string    `json:"region" yaml:"region"`
	NodeCount int       `json:"node_count" yaml:"node_count"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	Provider  string    `json:"provider" yaml:"provider"` // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-" yaml:"-"`
}

// IsActive returns true if the cluster is active
func (c *K8sCluster) IsActive() bool {
	return c.Status == "ACTIVE" || c.Status == "RUNNING"
}
