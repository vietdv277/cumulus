package gcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// SourceGCP is the only secret source on GCP contexts: Secret Manager is the
// sole backend, so the AWS-only selection flags and prefixes have nothing to
// select between.
const SourceGCP provider.SecretSource = "gcp"

// GCPSecretsProvider implements provider.SecretsProvider for GCP Secret Manager.
type GCPSecretsProvider struct {
	client *Client
	sm     *secretmanager.Service
}

// NewSecretsProvider creates a new GCP Secret Manager provider backed by the
// given Client.
func NewSecretsProvider(client *Client) *GCPSecretsProvider {
	return &GCPSecretsProvider{client: client}
}

// newService builds the Secret Manager REST client from the GCP client's ADC
// credentials; existing clients are reused so tests can inject a fake.
func (p *GCPSecretsProvider) newService(ctx context.Context) (*secretmanager.Service, error) {
	if p.sm != nil {
		return p.sm, nil
	}
	svc, err := secretmanager.NewService(ctx, option.WithTokenSource(p.client.Credentials().TokenSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}
	p.sm = svc
	return p.sm, nil
}

// List returns secrets in the context's project, filtered by name prefix.
func (p *GCPSecretsProvider) List(ctx context.Context, filter *provider.SecretFilter) ([]types.Secret, error) {
	if filter != nil && filter.Source != provider.SourceAll && filter.Source != SourceGCP {
		return nil, fmt.Errorf("%q is not a valid secret source for GCP: Secret Manager is the only backend", filter.Source)
	}

	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}

	var secrets []types.Secret
	err = svc.Projects.Secrets.List("projects/"+p.client.Project()).Pages(ctx, func(page *secretmanager.ListSecretsResponse) error {
		for _, s := range page.Secrets {
			name := secretShortName(s.Name)
			if filter != nil && filter.Prefix != "" && !strings.HasPrefix(name, filter.Prefix) {
				continue
			}
			secret := types.Secret{
				Name:     name,
				ARN:      s.Name,
				Provider: "gcp",
			}
			if s.CreateTime != "" {
				if t, err := time.Parse(time.RFC3339, s.CreateTime); err == nil {
					secret.CreatedAt = t
				}
			}
			secrets = append(secrets, secret)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	return secrets, nil
}

// Get returns a secret value, base64-decoding the latest version payload.
func (p *GCPSecretsProvider) Get(ctx context.Context, name string) (*types.SecretValue, error) {
	if prefix, ok := forcedBackendPrefix(name); ok {
		return nil, forcedBackendPrefixErr(name, prefix)
	}

	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := svc.Projects.Secrets.Versions.Access(secretFullName(p.client.Project(), name) + "/versions/latest").Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil, notFoundErr("secret", name)
		}
		return nil, fmt.Errorf("failed to get secret %s: %w", name, err)
	}

	value, err := base64.StdEncoding.DecodeString(resp.Payload.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode secret %s payload: %w", name, err)
	}

	return &types.SecretValue{
		Secret: types.Secret{
			Name:     name,
			ARN:      resp.Name,
			Provider: "gcp",
		},
		Value:   string(value),
		Version: secretShortName(resp.Name),
	}, nil
}

// Set adds a secret version, creating the secret first when it is missing.
func (p *GCPSecretsProvider) Set(ctx context.Context, name, value string) error {
	if prefix, ok := forcedBackendPrefix(name); ok {
		return forcedBackendPrefixErr(name, prefix)
	}

	svc, err := p.newService(ctx)
	if err != nil {
		return err
	}

	if err := p.addVersion(ctx, svc, name, value); err != nil {
		if !isNotFound(err) {
			return fmt.Errorf("failed to add version to secret %s: %w", name, err)
		}
		// Only a missing secret may create it; every other error surfaces.
		if err := p.createSecret(ctx, svc, name); err != nil {
			return err
		}
		if err := p.addVersion(ctx, svc, name, value); err != nil {
			return fmt.Errorf("failed to add version to secret %s: %w", name, err)
		}
	}
	return nil
}

// Delete removes the secret. GCP has no recovery window, so --force is
// accepted for CLI parity but changes nothing.
func (p *GCPSecretsProvider) Delete(ctx context.Context, name string, opts provider.DeleteSecretOptions) error {
	_ = opts
	if prefix, ok := forcedBackendPrefix(name); ok {
		return forcedBackendPrefixErr(name, prefix)
	}

	svc, err := p.newService(ctx)
	if err != nil {
		return err
	}

	if _, err := svc.Projects.Secrets.Delete(secretFullName(p.client.Project(), name)).Context(ctx).Do(); err != nil {
		if isNotFound(err) {
			return notFoundErr("secret", name)
		}
		return fmt.Errorf("failed to delete secret %s: %w", name, err)
	}
	return nil
}

// addVersion appends a new version with the base64-encoded payload.
func (p *GCPSecretsProvider) addVersion(ctx context.Context, svc *secretmanager.Service, name, value string) error {
	req := &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{
			Data: base64.StdEncoding.EncodeToString([]byte(value)),
		},
	}
	if _, err := svc.Projects.Secrets.AddVersion(secretFullName(p.client.Project(), name), req).Context(ctx).Do(); err != nil {
		return err
	}
	return nil
}

// createSecret creates the secret with automatic replication.
func (p *GCPSecretsProvider) createSecret(ctx context.Context, svc *secretmanager.Service, name string) error {
	secret := &secretmanager.Secret{
		Replication: &secretmanager.Replication{
			Automatic: &secretmanager.Automatic{},
		},
	}
	if _, err := svc.Projects.Secrets.Create("projects/"+p.client.Project(), secret).SecretId(name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to create secret %s: %w", name, err)
	}
	return nil
}

// secretFullName builds the resource name of a secret in the context project.
func secretFullName(project, name string) string {
	return fmt.Sprintf("projects/%s/secrets/%s", project, name)
}

// secretShortName returns the last path segment of a secret resource name.
func secretShortName(resource string) string {
	if i := strings.LastIndex(resource, "/"); i >= 0 {
		return resource[i+1:]
	}
	return resource
}

// forcedBackendPrefix reports whether name forces an AWS backend with an
// ssm:/sm: prefix; neither backend exists on GCP.
func forcedBackendPrefix(name string) (string, bool) {
	if strings.HasPrefix(name, "ssm:") {
		return "ssm", true
	}
	if strings.HasPrefix(name, "sm:") {
		return "sm", true
	}
	return "", false
}

// forcedBackendPrefixErr builds the error for an AWS-only backend prefix on a
// GCP context.
func forcedBackendPrefixErr(name, backend string) error {
	return fmt.Errorf("%q uses the AWS-only %s: backend prefix: GCP contexts only have Secret Manager", name, backend)
}
