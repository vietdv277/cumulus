package provider

import (
	"context"
	"errors"

	"github.com/vietdv277/cumulus/pkg/types"
)

// Common errors
var (
	ErrNotSupported    = errors.New("feature not supported by this provider")
	ErrNotFound        = errors.New("resource not found")
	ErrNotConfigured   = errors.New("provider not configured")
	ErrAuthFailed      = errors.New("authentication failed")
	ErrPermissionDenied = errors.New("permission denied")
)

// VMFilter contains filters for VM listing
type VMFilter struct {
	State   string            // running, stopped, etc.
	Name    string            // Name pattern
	Tags    map[string]string // Tag filters
}

// VMProvider defines the interface for VM operations
type VMProvider interface {
	// List returns VMs matching the filter
	List(ctx context.Context, filter *VMFilter) ([]types.VM, error)

	// Get returns a single VM by name or ID
	Get(ctx context.Context, nameOrID string) (*types.VM, error)

	// Start starts a VM
	Start(ctx context.Context, nameOrID string) error

	// Stop stops a VM
	Stop(ctx context.Context, nameOrID string) error

	// Reboot reboots a VM
	Reboot(ctx context.Context, nameOrID string) error

	// Connect establishes an interactive session to the VM
	Connect(ctx context.Context, nameOrID string) error

	// Tunnel creates a port forwarding tunnel to the VM
	Tunnel(ctx context.Context, nameOrID string, opts *TunnelOptions) error
}

// TunnelOptions contains options for creating a tunnel
type TunnelOptions struct {
	LocalPort  int
	RemotePort int
	RemoteHost string // For remote host forwarding
}

// SecretFilter contains filters for secret listing
type SecretFilter struct {
	Prefix string
}

// SecretsProvider defines the interface for secrets operations
type SecretsProvider interface {
	// List returns secrets matching the filter
	List(ctx context.Context, filter *SecretFilter) ([]types.Secret, error)

	// Get returns a secret value
	Get(ctx context.Context, name string) (*types.SecretValue, error)

	// Set creates or updates a secret
	Set(ctx context.Context, name string, value string) error

	// Delete removes a secret
	Delete(ctx context.Context, name string) error
}

// DBFilter contains filters for database listing
type DBFilter struct {
	Engine string // mysql, postgres, etc.
}

// DBProvider defines the interface for database operations
type DBProvider interface {
	// List returns databases matching the filter
	List(ctx context.Context, filter *DBFilter) ([]types.Database, error)

	// Get returns a single database by name or ID
	Get(ctx context.Context, nameOrID string) (*types.Database, error)

	// Connect establishes a connection or tunnel to the database
	Connect(ctx context.Context, nameOrID string, opts *DBConnectOptions) error
}

// DBConnectOptions contains options for connecting to a database
type DBConnectOptions struct {
	Via       string // Bastion/jump host
	LocalPort int
}

// StorageProvider defines the interface for object storage operations
type StorageProvider interface {
	// ListBuckets returns all buckets
	ListBuckets(ctx context.Context) ([]types.Bucket, error)

	// ListObjects returns objects in a bucket with optional prefix
	ListObjects(ctx context.Context, bucket, prefix string) ([]types.Object, error)

	// Copy copies an object
	Copy(ctx context.Context, src, dst string) error

	// Sync synchronizes objects between locations
	Sync(ctx context.Context, src, dst string, opts *SyncOptions) error

	// Presign generates a presigned URL
	Presign(ctx context.Context, path string, expiry int) (string, error)
}

// SyncOptions contains options for sync operation
type SyncOptions struct {
	Delete bool // Delete files in destination not in source
	DryRun bool // Don't actually copy
}

// LogsProvider defines the interface for log operations
type LogsProvider interface {
	// Tail streams logs from a target
	Tail(ctx context.Context, target string, opts *LogsOptions) error

	// Query queries logs
	Query(ctx context.Context, target string, opts *LogsOptions) ([]types.LogEntry, error)
}

// LogsOptions contains options for log operations
type LogsOptions struct {
	Follow bool   // Tail mode
	Since  string // Time range (e.g., "1h", "30m")
	Filter string // Filter pattern
	Limit  int    // Max entries
}

// K8sProvider defines the interface for Kubernetes operations
type K8sProvider interface {
	// ListClusters returns all K8s clusters
	ListClusters(ctx context.Context) ([]types.K8sCluster, error)

	// GetCluster returns a single cluster
	GetCluster(ctx context.Context, nameOrID string) (*types.K8sCluster, error)

	// UpdateKubeconfig updates kubeconfig for the cluster
	UpdateKubeconfig(ctx context.Context, nameOrID string) error
}

// CloudProvider is the main interface that all cloud providers must implement
type CloudProvider interface {
	// Name returns the provider identifier (e.g., "aws", "gcp")
	Name() string

	// IsConfigured returns true if the provider has valid credentials
	IsConfigured() bool

	// VM returns the VM provider
	VM() VMProvider

	// Secrets returns the secrets provider
	Secrets() SecretsProvider

	// DB returns the database provider (optional, may return nil)
	DB() DBProvider

	// Storage returns the storage provider (optional, may return nil)
	Storage() StorageProvider

	// Logs returns the logs provider (optional, may return nil)
	Logs() LogsProvider

	// K8s returns the Kubernetes provider (optional, may return nil)
	K8s() K8sProvider
}
