package gcp

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
)

const scopeCloudPlatform = "https://www.googleapis.com/auth/cloud-platform"

// Client wraps GCP credentials and configuration.
// It is the entry point for all GCP operations and holds Application Default
// Credentials loaded via google.FindDefaultCredentials.
type Client struct {
	credentials *google.Credentials
	project     string
	region      string
	ctx         context.Context
}

// Option is a functional option for configuring a Client.
type Option func(*Client)

// WithProject sets the GCP project ID.
func WithProject(project string) Option {
	return func(c *Client) {
		c.project = project
	}
}

// WithRegion sets the GCP region or zone.
func WithRegion(region string) Option {
	return func(c *Client) {
		c.region = region
	}
}

// NewClient creates a new GCP client using Application Default Credentials (ADC).
// ADC is resolved in this order:
//  1. GOOGLE_APPLICATION_CREDENTIALS environment variable (service account key file)
//  2. gcloud user credentials (~/.config/gcloud/application_default_credentials.json)
//  3. Metadata server (when running on GCE / GKE / Cloud Run)
//
// Returns an error with a helpful message if no credentials are found.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	c := &Client{ctx: ctx}
	for _, opt := range opts {
		opt(c)
	}

	creds, err := google.FindDefaultCredentials(ctx, scopeCloudPlatform)
	if err != nil {
		return nil, fmt.Errorf(
			"no GCP application default credentials found "+
				"(run 'gcloud auth application-default login'): %w",
			err,
		)
	}

	c.credentials = creds

	// Prefer the project from credentials when caller did not set one
	if c.project == "" && creds.ProjectID != "" {
		c.project = creds.ProjectID
	}

	return c, nil
}

// Project returns the configured GCP project ID.
func (c *Client) Project() string {
	return c.project
}

// Region returns the configured GCP region or zone.
func (c *Client) Region() string {
	return c.region
}

// Credentials returns the underlying google.Credentials.
// Consumers can use Credentials().TokenSource to build authenticated clients.
func (c *Client) Credentials() *google.Credentials {
	return c.credentials
}

// Context returns the context associated with this client.
func (c *Client) Context() context.Context {
	return c.ctx
}
