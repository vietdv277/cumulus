package types

import "time"

// K8sCluster represents a Kubernetes cluster
type K8sCluster struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`   // Kubernetes version
	Status    string    `json:"status"`    // ACTIVE, CREATING, etc.
	Endpoint  string    `json:"endpoint"`  // API server endpoint
	Region    string    `json:"region"`
	NodeCount int       `json:"node_count"`
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"` // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-"`
}

// IsActive returns true if the cluster is active
func (c *K8sCluster) IsActive() bool {
	return c.Status == "ACTIVE" || c.Status == "RUNNING"
}
