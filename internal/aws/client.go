package aws

import (
	"context"
	"fmt"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client wraps AWS SDK clients. Sub-clients are constructed lazily on first
// access so short-lived CLI invocations only pay for what they use.
type Client struct {
	cfg     awsconfig.Config
	ctx     context.Context
	profile string
	region  string

	ec2Once   sync.Once
	ec2Client *ec2.Client

	asgOnce   sync.Once
	asgClient *autoscaling.Client

	elbv2Once   sync.Once
	elbv2Client *elbv2.Client

	rdsOnce   sync.Once
	rdsClient *rds.Client

	s3Once   sync.Once
	s3Client *s3.Client

	eksOnce   sync.Once
	eksClient *eks.Client
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

// NewClient creates a new AWS Client with the given options. Only the shared
// AWS config is loaded here; service sub-clients are built on first use.
func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	c := &Client{
		ctx: ctx,
	}

	for _, opt := range opts {
		opt(c)
	}

	var configOpts []func(*config.LoadOptions) error

	if c.profile != "" {
		configOpts = append(configOpts, config.WithSharedConfigProfile(c.profile))
	}

	if c.region != "" {
		configOpts = append(configOpts, config.WithRegion(c.region))
	}

	cfg, err := config.LoadDefaultConfig(c.ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS SDK config: %w", err)
	}

	c.cfg = cfg
	return c, nil
}

// EC2 returns the lazily-constructed EC2 client.
func (c *Client) EC2() *ec2.Client {
	c.ec2Once.Do(func() { c.ec2Client = ec2.NewFromConfig(c.cfg) })
	return c.ec2Client
}

// ASG returns the lazily-constructed Auto Scaling client.
func (c *Client) ASG() *autoscaling.Client {
	c.asgOnce.Do(func() { c.asgClient = autoscaling.NewFromConfig(c.cfg) })
	return c.asgClient
}

// ELBv2 returns the lazily-constructed ELBv2 client.
func (c *Client) ELBv2() *elbv2.Client {
	c.elbv2Once.Do(func() { c.elbv2Client = elbv2.NewFromConfig(c.cfg) })
	return c.elbv2Client
}

// RDS returns the lazily-constructed RDS client.
func (c *Client) RDS() *rds.Client {
	c.rdsOnce.Do(func() { c.rdsClient = rds.NewFromConfig(c.cfg) })
	return c.rdsClient
}

// S3 returns the lazily-constructed S3 client.
func (c *Client) S3() *s3.Client {
	c.s3Once.Do(func() { c.s3Client = s3.NewFromConfig(c.cfg) })
	return c.s3Client
}

// EKS returns the lazily-constructed EKS client.
func (c *Client) EKS() *eks.Client {
	c.eksOnce.Do(func() { c.eksClient = eks.NewFromConfig(c.cfg) })
	return c.eksClient
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
