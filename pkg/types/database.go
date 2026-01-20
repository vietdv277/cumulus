package types

import "time"

// Database represents a managed database instance
type Database struct {
	ID        string    `json:"id"`        // Provider-specific ID
	Name      string    `json:"name"`      // Database name
	Engine    string    `json:"engine"`    // mysql, postgres, etc.
	Version   string    `json:"version"`   // Engine version
	Endpoint  string    `json:"endpoint"`  // Connection endpoint
	Port      int       `json:"port"`      // Connection port
	State     string    `json:"state"`     // available, stopped, etc.
	Size      string    `json:"size"`      // Instance size
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"` // aws, gcp

	// Raw holds the original API response
	Raw interface{} `json:"-"`
}

// IsAvailable returns true if the database is available
func (d *Database) IsAvailable() bool {
	return d.State == "available" || d.State == "running"
}
