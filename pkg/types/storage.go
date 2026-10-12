package types

import "time"

// Bucket represents a storage bucket
type Bucket struct {
	Name      string    `json:"name" yaml:"name"`
	Region    string    `json:"region" yaml:"region"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	Provider  string    `json:"provider" yaml:"provider"` // aws, gcp
}

// Object represents an object in storage
type Object struct {
	Key          string    `json:"key" yaml:"key"`
	Size         int64     `json:"size" yaml:"size"`
	LastModified time.Time `json:"last_modified" yaml:"last_modified"`
	ETag         string    `json:"etag" yaml:"etag"`
	StorageClass string    `json:"storage_class" yaml:"storage_class"`
}
