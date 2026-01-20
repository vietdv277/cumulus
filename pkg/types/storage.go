package types

import "time"

// Bucket represents a storage bucket
type Bucket struct {
	Name      string    `json:"name"`
	Region    string    `json:"region"`
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"` // aws, gcp
}

// Object represents an object in storage
type Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	StorageClass string    `json:"storage_class"`
}
