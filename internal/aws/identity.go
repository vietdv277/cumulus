package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CallerIdentity represents AWS caller identity information
type CallerIdentity struct {
	Account string
	Arn     string
	UserID  string
}

// GetCallerIdentity returns the current AWS caller identity
func GetCallerIdentity(profile, region string) (*CallerIdentity, error) {
	ctx := context.Background()

	var configOpts []func(*config.LoadOptions) error

	if profile != "" {
		configOpts = append(configOpts, config.WithSharedConfigProfile(profile))
	}

	if region != "" {
		configOpts = append(configOpts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return nil, err
	}

	stsClient := sts.NewFromConfig(cfg)

	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}

	return &CallerIdentity{
		Account: deref(output.Account),
		Arn:     deref(output.Arn),
		UserID:  deref(output.UserId),
	}, nil
}
