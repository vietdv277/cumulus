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
