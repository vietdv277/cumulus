package types

import "time"

// Database represents a managed database instance
type Database struct {
	ID        string    `json:"id" yaml:"id"`             // Provider-specific ID
	Name      string    `json:"name" yaml:"name"`         // Database name
	Engine    string    `json:"engine" yaml:"engine"`     // mysql, postgres, etc.
	Version   string    `json:"version" yaml:"version"`   // Engine version
	Endpoint  string    `json:"endpoint" yaml:"endpoint"` // Connection endpoint
	Port      int       `json:"port" yaml:"port"`         // Connection port
	State     string    `json:"state" yaml:"state"`       // available, stopped, etc.
	Size      string    `json:"size" yaml:"size"`         // Instance size
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	Provider  string    `json:"provider" yaml:"provider"` // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-" yaml:"-"`
}

// IsAvailable returns true if the database is available
func (d *Database) IsAvailable() bool {
	return d.State == "available" || d.State == "running"
}
