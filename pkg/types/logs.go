package types

import "time"

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`    // Log group/stream
	Level     string    `json:"level"`     // INFO, ERROR, etc.
	Provider  string    `json:"provider"`
}
