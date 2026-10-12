package gcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// GCPDBProvider implements provider.DBProvider for Cloud SQL.
type GCPDBProvider struct {
	client *Client
	sql    *sqladmin.Service
}

// NewDBProvider creates a new Cloud SQL provider backed by the given Client.
func NewDBProvider(client *Client) *GCPDBProvider {
	return &GCPDBProvider{client: client}
}

// newService builds the SQL Admin REST client from the GCP client's ADC
// credentials; existing clients are reused so tests can inject a fake.
func (p *GCPDBProvider) newService(ctx context.Context) (*sqladmin.Service, error) {
	if p.sql != nil {
		return p.sql, nil
	}
	svc, err := sqladmin.NewService(ctx, option.WithTokenSource(p.client.Credentials().TokenSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create SQL Admin client: %w", err)
	}
	p.sql = svc
	return p.sql, nil
}

// List returns Cloud SQL instances in the context's project.
func (p *GCPDBProvider) List(ctx context.Context, filter *provider.DBFilter) ([]types.Database, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}

	var dbs []types.Database
	if err := svc.Instances.List(p.client.Project()).Pages(ctx, func(page *sqladmin.InstancesListResponse) error {
		for _, inst := range page.Items {
			db := cloudSQLToDatabase(inst)
			if filter != nil && filter.Engine != "" && !strings.EqualFold(db.Engine, filter.Engine) {
				continue
			}
			dbs = append(dbs, db)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list Cloud SQL instances: %w", err)
	}
	return dbs, nil
}

// Get returns a single Cloud SQL instance by name.
func (p *GCPDBProvider) Get(ctx context.Context, nameOrID string) (*types.Database, error) {
	svc, err := p.newService(ctx)
	if err != nil {
		return nil, err
	}

	inst, err := svc.Instances.Get(p.client.Project(), nameOrID).Do()
	if err != nil {
		if isNotFound(err) {
			return nil, notFoundErr("Cloud SQL instance", nameOrID)
		}
		return nil, fmt.Errorf("failed to get Cloud SQL instance %s: %w", nameOrID, err)
	}

	db := cloudSQLToDatabase(inst)
	return &db, nil
}

// Connect opens a local tunnel to the instance with cloud-sql-proxy.
func (p *GCPDBProvider) Connect(ctx context.Context, nameOrID string, opts *provider.DBConnectOptions) error {
	if opts != nil && opts.Via != "" {
		return fmt.Errorf("--via is not supported for GCP: cloud-sql-proxy connects to the instance directly")
	}

	db, err := p.Get(ctx, nameOrID)
	if err != nil {
		return err
	}

	localPort := 0
	if opts != nil {
		localPort = opts.LocalPort
	}
	if localPort == 0 {
		port, err := defaultDBPort(db.Engine)
		if err != nil {
			return err
		}
		localPort = port
	}

	proxy, err := exec.LookPath("cloud-sql-proxy")
	if err != nil {
		return fmt.Errorf("cloud-sql-proxy not found on PATH — install it (see https://cloud.google.com/sql/docs/mysql/sql-proxy): %w", err)
	}

	cmd := exec.CommandContext(ctx, proxy, db.Endpoint, "--port", strconv.Itoa(localPort))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Tunneling %s -> localhost:%d via cloud-sql-proxy\n", db.Endpoint, localPort)
	fmt.Println("Press Ctrl+C to close the tunnel")

	if err := startTunnelProc(cmd); err != nil {
		return fmt.Errorf("failed to start cloud-sql-proxy: %w", err)
	}

	waitDone := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		err := cmd.Wait()
		waitDone <- err
		close(exit)
	}()
	defer stopTunnel(cmd, exit)

	// The proxy runs in its own process group, so Ctrl-C reaches only cml;
	// without this handler cml would die on the default SIGINT action and
	// orphan the proxy. Tear the group down on our own terms instead.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-waitDone:
		return err
	case <-sigCh:
		stopTunnel(cmd, exit)
		return nil
	}
}

// startTunnelProc starts a prepared tunnel command in its own process group
// (unix) so the proxy's children can be signalled with it.
func startTunnelProc(cmd *exec.Cmd) error {
	tunnelProcGroup(cmd)
	return cmd.Start()
}

// stopTunnel signals the tunnel's process group, escalating to a kill after a
// short grace period. exit must be closed by the Wait watcher.
func stopTunnel(cmd *exec.Cmd, exit <-chan struct{}) {
	if cmd.Process == nil {
		return
	}
	_ = TerminateTunnel(cmd)
	select {
	case <-exit:
	case <-time.After(3 * time.Second):
		_ = KillTunnel(cmd)
		<-exit
	}
}

// cloudSQLToDatabase converts a Cloud SQL instance to the unified Database type.
func cloudSQLToDatabase(i *sqladmin.DatabaseInstance) types.Database {
	engine, version := parseDatabaseVersion(i.DatabaseVersion)
	db := types.Database{
		ID:       i.Name,
		Name:     i.Name,
		Engine:   engine,
		Version:  version,
		Endpoint: i.ConnectionName,
		State:    cloudSQLState(i.State),
		Provider: "gcp",
		Raw:      i,
	}
	if port, err := defaultDBPort(engine); err == nil {
		db.Port = port
	}
	if i.Settings != nil {
		db.Size = i.Settings.Tier
	}
	if t, err := time.Parse(time.RFC3339, i.CreateTime); err == nil {
		db.CreatedAt = t
	}
	return db
}

// parseDatabaseVersion splits a databaseVersion like "POSTGRES_15" into engine
// and version ("postgres", "15"). Minor versions keep their components joined
// with dots ("MYSQL_8_0_18" → "8.0.18").
func parseDatabaseVersion(databaseVersion string) (engine, version string) {
	parts := strings.SplitN(databaseVersion, "_", 2)
	engine = strings.ToLower(parts[0])
	if len(parts) > 1 {
		version = strings.ToLower(strings.ReplaceAll(parts[1], "_", "."))
	}
	return engine, version
}

// defaultDBPort maps an engine to its default local tunnel port.
func defaultDBPort(engine string) (int, error) {
	switch engine {
	case "mysql":
		return 3306, nil
	case "postgres":
		return 5432, nil
	case "sqlserver":
		return 1433, nil
	default:
		return 0, fmt.Errorf("unknown Cloud SQL engine %q: pass --local-port", engine)
	}
}

// cloudSQLState maps a Cloud SQL instance state to the unified states used by
// `cml db` (available, stopped, creating, ...).
func cloudSQLState(state string) string {
	switch state {
	case "RUNNABLE":
		return "available"
	case "SUSPENDED":
		return "stopped"
	case "PENDING_CREATE":
		return "creating"
	case "PENDING_DELETE":
		return "deleting"
	case "MAINTENANCE":
		return "maintenance"
	case "FAILED":
		return "failed"
	default:
		return strings.ToLower(state)
	}
}
