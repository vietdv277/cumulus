package aws

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Client wraps AWS SDK clients
type Client struct {
	EC2     *ec2.Client
	SSM     *ssm.Client
	ASG     *autoscaling.Client
	ELBv2   *elbv2.Client
	cfg     awsconfig.Config
	ctx     context.Context
	profile string
	region  string
}

// ClientOption allows customizing the AWS Client
type ClientOption func(*Client)

// WithProfile sets the AWS profile for the client
func WithProfile(profile string) ClientOption {
	return func(c *Client) {
		c.profile = profile
	}
}

// WithRegion sets the AWS region for the client
func WithRegion(region string) ClientOption {
	return func(c *Client) {
		c.region = region
	}
}

// NewClient creates a new AWS Client with the given options
func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	c := &Client{
		ctx: ctx,
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Build config options
	var configOpts []func(*config.LoadOptions) error

	if c.profile != "" {
		configOpts = append(configOpts, config.WithSharedConfigProfile(c.profile))
	}

	if c.region != "" {
		configOpts = append(configOpts, config.WithRegion(c.region))
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(c.ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS SDK config: %w", err)
	}

	c.cfg = cfg
	c.EC2 = ec2.NewFromConfig(cfg)
	c.SSM = ssm.NewFromConfig(cfg)
	c.ASG = autoscaling.NewFromConfig(cfg)
	c.ELBv2 = elbv2.NewFromConfig(cfg)

	return c, nil
}

// Config returns the underlying AWS config
func (c *Client) Config() awsconfig.Config {
	return c.cfg
}

// Context returns the client's context
func (c *Client) Context() context.Context {
	return c.ctx
}

// Profile returns the AWS profile
func (c *Client) Profile() string {
	return c.profile
}

// Region returns the AWS region
func (c *Client) Region() string {
	return c.region
}
