package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smTypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

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
	// Try SSM Parameter Store first (for paths starting with /)
	if len(name) > 0 && name[0] == '/' {
		return p.getSSMParameter(ctx, name)
	}

	// Try Secrets Manager
	return p.getSecretsManager(ctx, name)
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
	// Use SSM Parameter Store for paths starting with /
	if len(name) > 0 && name[0] == '/' {
		return p.setSSMParameter(ctx, name, value)
	}

	// Use Secrets Manager for other secrets
	return p.setSecretsManager(ctx, name, value)
}

func (p *AWSSecretsProvider) setSSMParameter(ctx context.Context, name, value string) error {
	_, err := p.ssm.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      &name,
		Value:     &value,
		Type:      ssmTypes.ParameterTypeSecureString,
		Overwrite: boolPtr(true),
	})
	return err
}

func (p *AWSSecretsProvider) setSecretsManager(ctx context.Context, name, value string) error {
	// Try to update existing secret first
	_, err := p.sm.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     &name,
		SecretString: &value,
	})
	if err != nil {
		// If secret doesn't exist, create it
		_, err = p.sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         &name,
			SecretString: &value,
		})
	}
	return err
}

// Delete removes a secret
func (p *AWSSecretsProvider) Delete(ctx context.Context, name string) error {
	// Use SSM Parameter Store for paths starting with /
	if len(name) > 0 && name[0] == '/' {
		_, err := p.ssm.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: &name,
		})
		return err
	}

	// Use Secrets Manager for other secrets
	_, err := p.sm.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   &name,
		ForceDeleteWithoutRecovery: boolPtr(true),
	})
	return err
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func safeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
