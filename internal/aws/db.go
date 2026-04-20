package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// AWSDBProvider implements the DBProvider interface for AWS RDS
type AWSDBProvider struct {
	client  *Client
	profile string
	region  string
}

// NewDBProvider creates a new AWS DB provider
func NewDBProvider(client *Client, profile, region string) *AWSDBProvider {
	return &AWSDBProvider{
		client:  client,
		profile: profile,
		region:  region,
	}
}

// List returns RDS instances matching the filter
func (p *AWSDBProvider) List(ctx context.Context, filter *provider.DBFilter) ([]types.Database, error) {
	input := &rds.DescribeDBInstancesInput{}

	if filter != nil && filter.Engine != "" {
		input.Filters = []rdstypes.Filter{{
			Name:   strPtr("engine"),
			Values: []string{filter.Engine},
		}}
	}

	var dbs []types.Database
	paginator := rds.NewDescribeDBInstancesPaginator(p.client.RDS(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe DB instances: %w", err)
		}
		for _, inst := range page.DBInstances {
			dbs = append(dbs, rdsToDatabase(inst))
		}
	}

	return dbs, nil
}

// Get returns a single database by identifier or endpoint name
func (p *AWSDBProvider) Get(ctx context.Context, nameOrID string) (*types.Database, error) {
	output, err := p.client.RDS().DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: strPtr(nameOrID),
	})
	if err != nil {
		return nil, fmt.Errorf("DB instance not found: %s", nameOrID)
	}
	if len(output.DBInstances) == 0 {
		return nil, fmt.Errorf("DB instance not found: %s", nameOrID)
	}
	db := rdsToDatabase(output.DBInstances[0])
	return &db, nil
}

// Connect opens a port-forwarding tunnel to the database via an SSM bastion.
// The bastion EC2 instance ID must be provided via opts.Via.
func (p *AWSDBProvider) Connect(ctx context.Context, nameOrID string, opts *provider.DBConnectOptions) error {
	db, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}
	if db.Endpoint == "" {
		return fmt.Errorf("database %s has no endpoint yet (state: %s)", db.Name, db.State)
	}

	if opts == nil || opts.Via == "" {
		return fmt.Errorf("RDS connect requires --via <bastion-instance-id> to tunnel through an SSM-enabled EC2 host")
	}

	localPort := opts.LocalPort
	if localPort == 0 {
		localPort = db.Port
	}

	params := map[string][]string{
		"host":            {db.Endpoint},
		"portNumber":      {strconv.Itoa(db.Port)},
		"localPortNumber": {strconv.Itoa(localPort)},
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	args := []string{
		"ssm", "start-session",
		"--target", opts.Via,
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", string(paramsJSON),
	}
	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}
	if p.region != "" {
		args = append(args, "--region", p.region)
	}

	fmt.Printf("Tunneling %s -> localhost:%d via %s\n", db.Endpoint, localPort, opts.Via)
	fmt.Println("Press Ctrl+C to close the tunnel")

	ssmCmd := exec.Command("aws", args...)
	ssmCmd.Stdin = os.Stdin
	ssmCmd.Stdout = os.Stdout
	ssmCmd.Stderr = os.Stderr
	return ssmCmd.Run()
}

// rdsToDatabase converts an RDS DBInstance to the unified Database type
func rdsToDatabase(i rdstypes.DBInstance) types.Database {
	db := types.Database{
		ID:       deref(i.DBInstanceIdentifier),
		Name:     deref(i.DBInstanceIdentifier),
		Engine:   deref(i.Engine),
		Version:  deref(i.EngineVersion),
		State:    strings.ToLower(deref(i.DBInstanceStatus)),
		Size:     deref(i.DBInstanceClass),
		Provider: "aws",
		Raw:      i,
	}

	if i.Endpoint != nil {
		db.Endpoint = deref(i.Endpoint.Address)
		if i.Endpoint.Port != nil {
			db.Port = int(*i.Endpoint.Port)
		}
	}

	if i.InstanceCreateTime != nil {
		db.CreatedAt = *i.InstanceCreateTime
	}

	return db
}
