package types

import "time"

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Message   string    `json:"message" yaml:"message"`
	Source    string    `json:"source" yaml:"source"` // Log group/stream
	Level     string    `json:"level" yaml:"level"`   // INFO, ERROR, etc.
	Provider  string    `json:"provider" yaml:"provider"`
}

// LogTarget is something logs can be read from: a CloudWatch log group (AWS)
// or a log name (GCP).
type LogTarget struct {
	Name      string    `json:"name" yaml:"name"`
	Provider  string    `json:"provider" yaml:"provider"`
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
}
