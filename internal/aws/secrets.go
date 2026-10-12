package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smTypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// SecretBackend identifies which AWS backend holds a secret.
type SecretBackend string

const (
	// BackendSSM is SSM Parameter Store.
	BackendSSM SecretBackend = "ssm"
	// BackendSM is AWS Secrets Manager.
	BackendSM SecretBackend = "sm"
)

// SecretsManagerRecoveryWindowDays is the default recovery window (in days)
// applied to Secrets Manager deletions.
const SecretsManagerRecoveryWindowDays = 7

// resolveSecretRoute maps a user-provided secret name to its backend. An
// explicit `ssm:`/`sm:` prefix forces the backend and is stripped. Without a
// prefix, a leading `/` routes to SSM; anything else routes to Secrets Manager
// but may still live in SSM, so callers fall back there when the secret is not
// found (fallbackSSM).
func resolveSecretRoute(name string) (SecretBackend, string, bool) {
	if n, ok := strings.CutPrefix(name, "ssm:"); ok {
		return BackendSSM, n, false
	}
	if n, ok := strings.CutPrefix(name, "sm:"); ok {
		return BackendSM, n, false
	}
	if strings.HasPrefix(name, "/") {
		return BackendSSM, name, false
	}
	return BackendSM, name, true
}

// ResolveSecretBackend returns the backend and the stripped name for a secret name.
func ResolveSecretBackend(name string) (SecretBackend, string) {
	backend, n, _ := resolveSecretRoute(name)
	return backend, n
}

// AWSSecretsProvider implements the SecretsProvider interface for AWS
// It supports both SSM Parameter Store and Secrets Manager
type AWSSecretsProvider struct {
	client  *Client
	ssm     *ssm.Client
	sm      *secretsmanager.Client
	profile string
	region  string
}

// NewSecretsProvider creates a new AWS Secrets provider
func NewSecretsProvider(client *Client, ssmClient *ssm.Client, smClient *secretsmanager.Client, profile, region string) *AWSSecretsProvider {
	return &AWSSecretsProvider{
		client:  client,
		ssm:     ssmClient,
		sm:      smClient,
		profile: profile,
		region:  region,
	}
}

// List returns secrets matching the filter
func (p *AWSSecretsProvider) List(ctx context.Context, filter *provider.SecretFilter) ([]types.Secret, error) {
	// Query only the selected backend so a missing permission on the other one
	// does not fail the whole listing.
	if filter != nil && filter.Source != "" && filter.Source != provider.SourceAll {
		switch filter.Source {
		case provider.SourceSSM:
			return p.listSSMParameters(ctx, filter)
		case provider.SourceSM:
			return p.listSecretsManager(ctx, filter)
		default:
			return nil, fmt.Errorf("unknown secret source: %s", filter.Source)
		}
	}

	var secrets []types.Secret

	// List from SSM Parameter Store
	ssmSecrets, err := p.listSSMParameters(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list SSM parameters: %w", err)
	}
	secrets = append(secrets, ssmSecrets...)

	// List from Secrets Manager
	smSecrets, err := p.listSecretsManager(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list Secrets Manager secrets: %w", err)
	}
	secrets = append(secrets, smSecrets...)

	return secrets, nil
}

func (p *AWSSecretsProvider) listSSMParameters(ctx context.Context, filter *provider.SecretFilter) ([]types.Secret, error) {
	input := &ssm.DescribeParametersInput{}

	if filter != nil && filter.Prefix != "" {
		input.ParameterFilters = []ssmTypes.ParameterStringFilter{
			{
				Key:    strPtr("Name"),
				Option: strPtr("BeginsWith"),
				Values: []string{filter.Prefix},
			},
		}
	}

	paginator := ssm.NewDescribeParametersPaginator(p.ssm, input)

	var secrets []types.Secret
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, param := range page.Parameters {
			secret := types.Secret{
				Name:     deref(param.Name),
				ARN:      deref(param.Name), // SSM doesn't have ARNs for parameters in this API
				Provider: "aws",
			}
			if param.LastModifiedDate != nil {
				secret.UpdatedAt = *param.LastModifiedDate
			}
			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

func (p *AWSSecretsProvider) listSecretsManager(ctx context.Context, filter *provider.SecretFilter) ([]types.Secret, error) {
	input := &secretsmanager.ListSecretsInput{}

	if filter != nil && filter.Prefix != "" {
		// Secrets Manager uses name prefix in filters
		input.Filters = []smTypes.Filter{
			{
				Key:    smTypes.FilterNameStringTypeName,
				Values: []string{filter.Prefix},
			},
		}
	}

	paginator := secretsmanager.NewListSecretsPaginator(p.sm, input)

	var secrets []types.Secret
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, s := range page.SecretList {
			secret := types.Secret{
				Name:     deref(s.Name),
				ARN:      deref(s.ARN),
				Provider: "aws",
			}
			if s.CreatedDate != nil {
				secret.CreatedAt = *s.CreatedDate
			}
			if s.LastChangedDate != nil {
				secret.UpdatedAt = *s.LastChangedDate
			}
			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

// Get returns a secret value
func (p *AWSSecretsProvider) Get(ctx context.Context, name string) (*types.SecretValue, error) {
	backend, n, fallbackSSM := resolveSecretRoute(name)

	if backend == BackendSSM {
		return p.getSSMParameter(ctx, n)
	}

	value, err := p.getSecretsManager(ctx, n)
	if err != nil {
		// Fall back to SSM by the same name when the secret is not in Secrets Manager.
		var notFound *smTypes.ResourceNotFoundException
		if errors.As(err, &notFound) && fallbackSSM {
			return p.getSSMParameter(ctx, n)
		}
		return nil, err
	}
	return value, nil
}

func (p *AWSSecretsProvider) getSSMParameter(ctx context.Context, name string) (*types.SecretValue, error) {
	output, err := p.ssm.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get SSM parameter: %w", err)
	}

	param := output.Parameter
	return &types.SecretValue{
		Secret: types.Secret{
			Name:      deref(param.Name),
			ARN:       deref(param.ARN),
			Provider:  "aws",
			UpdatedAt: safeTime(param.LastModifiedDate),
		},
		Value:   deref(param.Value),
		Version: fmt.Sprintf("%d", param.Version),
	}, nil
}

func (p *AWSSecretsProvider) getSecretsManager(ctx context.Context, name string) (*types.SecretValue, error) {
	output, err := p.sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	return &types.SecretValue{
		Secret: types.Secret{
			Name:      deref(output.Name),
			ARN:       deref(output.ARN),
			Provider:  "aws",
			CreatedAt: safeTime(output.CreatedDate),
		},
		Value:   deref(output.SecretString),
		Version: deref(output.VersionId),
	}, nil
}

// Set creates or updates a secret
func (p *AWSSecretsProvider) Set(ctx context.Context, name string, value string) error {
	backend, n, fallbackSSM := resolveSecretRoute(name)

	if backend == BackendSSM {
		return p.setSSMParameter(ctx, n, value)
	}

	if fallbackSSM {
		// Update the existing SSM parameter (keeping its type) instead of
		// creating a duplicate Secrets Manager secret.
		existing, err := p.ssm.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           &n,
			WithDecryption: boolPtr(true),
		})
		if err != nil {
			var notFound *ssmTypes.ParameterNotFound
			if !errors.As(err, &notFound) {
				return fmt.Errorf("failed to check SSM parameter %s: %w", n, err)
			}
			// Not in SSM — create in Secrets Manager below.
		} else if existing.Parameter != nil {
			paramType := existing.Parameter.Type
			if paramType == "" {
				paramType = ssmTypes.ParameterTypeSecureString
			}
			_, err = p.ssm.PutParameter(ctx, &ssm.PutParameterInput{
				Name:      &n,
				Value:     &value,
				Type:      paramType,
				Overwrite: boolPtr(true),
			})
			if err != nil {
				return fmt.Errorf("failed to update SSM parameter %s: %w", n, err)
			}
			return nil
		}
	}

	// Use Secrets Manager
	return p.setSecretsManager(ctx, n, value)
}

func (p *AWSSecretsProvider) setSSMParameter(ctx context.Context, name, value string) error {
	_, err := p.ssm.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      &name,
		Value:     &value,
		Type:      ssmTypes.ParameterTypeSecureString,
		Overwrite: boolPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to set SSM parameter %s: %w", name, err)
	}
	return nil
}

func (p *AWSSecretsProvider) setSecretsManager(ctx context.Context, name, value string) error {
	// Try to update existing secret first
	_, err := p.sm.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     &name,
		SecretString: &value,
	})
	if err != nil {
		// Only create when the secret doesn't exist; anything else
		// (permissions, throttling, pending deletion) must surface as-is.
		var notFound *smTypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			_, err = p.sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         &name,
				SecretString: &value,
			})
			if err != nil {
				return fmt.Errorf("failed to create secret %s: %w", name, err)
			}
			return nil
		}
		return fmt.Errorf("failed to update secret %s: %w", name, err)
	}
	return nil
}

// Delete removes a secret. Plain names are deleted from Secrets Manager only:
// when only an SSM parameter of the same name exists, nothing is deleted and an
// error points at the explicit ssm: prefix.
func (p *AWSSecretsProvider) Delete(ctx context.Context, name string, opts provider.DeleteSecretOptions) error {
	backend, n, fallbackSSM := resolveSecretRoute(name)

	if backend == BackendSSM {
		return p.deleteSSMParameter(ctx, n)
	}

	input := &secretsmanager.DeleteSecretInput{SecretId: &n}
	if opts.Force {
		input.ForceDeleteWithoutRecovery = boolPtr(true)
	} else {
		input.RecoveryWindowInDays = int64Ptr(SecretsManagerRecoveryWindowDays)
	}

	_, err := p.sm.DeleteSecret(ctx, input)
	if err != nil {
		var notFound *smTypes.ResourceNotFoundException
		// Never delete an SSM parameter behind a Secrets Manager not-found.
		if errors.As(err, &notFound) && fallbackSSM {
			return p.ssmDeleteConflict(ctx, n)
		}
		return fmt.Errorf("failed to delete secret %s: %w", n, err)
	}
	return nil
}

// ssmDeleteConflict reports what happened when a plain-name Secrets Manager
// delete found no secret: either nothing exists anywhere, or an SSM parameter
// of the same name exists and the user must opt in via the ssm: prefix.
func (p *AWSSecretsProvider) ssmDeleteConflict(ctx context.Context, name string) error {
	_, err := p.ssm.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		var notFound *ssmTypes.ParameterNotFound
		if errors.As(err, &notFound) {
			return fmt.Errorf("secret %q not found: %w", name, provider.ErrNotFound)
		}
		return fmt.Errorf("failed to check SSM parameter %s: %w", name, err)
	}
	return ssmDeleteHint(name)
}

// ssmDeleteHint builds the error telling the user an SSM parameter of the same
// name exists and must be deleted explicitly.
func ssmDeleteHint(name string) error {
	return fmt.Errorf("secret %q not found in Secrets Manager; an SSM parameter with that name exists — run 'cml secrets delete ssm:%s' to delete it permanently", name, name)
}

func (p *AWSSecretsProvider) deleteSSMParameter(ctx context.Context, name string) error {
	_, err := p.ssm.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: &name,
	})
	if err != nil {
		return fmt.Errorf("failed to delete SSM parameter %s: %w", name, err)
	}
	return nil
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

func safeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
