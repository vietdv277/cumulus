package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage managed databases",
	Long: `Manage managed databases (RDS / Cloud SQL) within the current context.

Use 'cml use <context>' to switch contexts, or pass --context to override per-command.

Examples:
  cml db list                          # List databases
  cml db list --engine postgres        # Filter by engine
  cml db get prod-pg                   # Show details
  cml db connect prod-pg --via bastion # Tunnel via SSM bastion`,
}

var dbListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List databases",
	RunE:    runDBList,
}

var dbGetCmd = &cobra.Command{
	Use:   "get <name-or-id>",
	Short: "Get database details",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBGet,
}

var dbConnectCmd = &cobra.Command{
	Use:   "connect <name-or-id>",
	Short: "Open a port-forwarding tunnel to a database",
	Long: `Open a tunnel to the database endpoint.

For AWS RDS, --via must specify a bastion EC2 instance ID that has the SSM
agent installed; the tunnel uses AWS-StartPortForwardingSessionToRemoteHost.

Examples:
  cml db connect prod-pg --via i-0abc123def456
  cml db connect prod-pg --via i-0abc123def456 --local-port 15432`,
	Args: cobra.ExactArgs(1),
	RunE: runDBConnect,
}

var (
	dbListEngine   string
	dbConnectVia   string
	dbConnectLocal int
	dbContextFlag  string
)

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbListCmd)
	dbCmd.AddCommand(dbGetCmd)
	dbCmd.AddCommand(dbConnectCmd)

	dbListCmd.Flags().StringVar(&dbListEngine, "engine", "", "Filter by engine (mysql, postgres, ...)")

	dbConnectCmd.Flags().StringVar(&dbConnectVia, "via", "", "Bastion instance ID for SSM port forwarding")
	dbConnectCmd.Flags().IntVar(&dbConnectLocal, "local-port", 0, "Local port (defaults to remote port)")

	dbCmd.PersistentFlags().StringVarP(&dbContextFlag, "context", "c", "", "Use specific context")
}

// getDBProvider returns a DBProvider for the active or overridden context.
func getDBProvider(ctx context.Context) (provider.DBProvider, error) {
	var ctxConfig *config.Context
	var ctxName string
	var err error

	if dbContextFlag != "" {
		cfg, loadErr := config.LoadCMLConfig()
		if loadErr != nil {
			return nil, loadErr
		}
		ctxConfig = cfg.Contexts[dbContextFlag]
		if ctxConfig == nil {
			return nil, fmt.Errorf("context %q not found", dbContextFlag)
		}
		ctxName = dbContextFlag
	} else {
		ctxConfig, ctxName, err = config.GetCurrentContext()
		if err != nil {
			return nil, err
		}
		if ctxConfig == nil {
			return nil, fmt.Errorf("no context set. Use 'cml use <context>' to set one")
		}
	}

	switch ctxConfig.Provider {
	case "aws":
		client, err := aws.NewClient(ctx,
			aws.WithProfile(ctxConfig.Profile),
			aws.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create AWS client: %w", err)
		}
		return aws.NewDBProvider(client, ctxConfig.Profile, ctxConfig.Region), nil

	case "gcp":
		return nil, fmt.Errorf("db commands are not yet implemented for GCP (context: %s)", ctxName)

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

func runDBList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dbProvider, err := getDBProvider(ctx)
	if err != nil {
		return err
	}

	filter := &provider.DBFilter{Engine: dbListEngine}
	dbs, err := dbProvider.List(ctx, filter)
	if err != nil {
		return err
	}

	if len(dbs) == 0 {
		fmt.Println("No databases found")
		return nil
	}

	printDBTable(dbs)
	return nil
}

func runDBGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dbProvider, err := getDBProvider(ctx)
	if err != nil {
		return err
	}

	db, err := dbProvider.Get(ctx, args[0])
	if err != nil {
		return err
	}

	printDBDetails(db)
	return nil
}

func runDBConnect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dbProvider, err := getDBProvider(ctx)
	if err != nil {
		return err
	}

	opts := &provider.DBConnectOptions{
		Via:       dbConnectVia,
		LocalPort: dbConnectLocal,
	}
	return dbProvider.Connect(ctx, args[0], opts)
}

func printDBTable(dbs []types.Database) {
	headers := []string{"Name", "Engine", "Version", "State", "Endpoint", "Port", "Size"}
	widths := []int{28, 12, 10, 12, 50, 6, 18}

	var sb strings.Builder

	sb.WriteString(ui.BorderStyle.Render(ui.TopLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.TopT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.TopRight))
	sb.WriteString("\n")

	sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	for i, h := range headers {
		cell := " " + padRightDB(h, widths[i]) + " "
		sb.WriteString(ui.HeaderStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	}
	sb.WriteString("\n")

	sb.WriteString(ui.BorderStyle.Render(ui.LeftT))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.Cross))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.RightT))
	sb.WriteString("\n")

	for _, db := range dbs {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell := " " + padRightDB(db.Name, widths[0]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightDB(db.Engine, widths[1]) + " "
		sb.WriteString(ui.TypeStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightDB(db.Version, widths[2]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		sb.WriteString(formatDBState(db.State, widths[3]))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightDB(db.Endpoint, widths[4]) + " "
		sb.WriteString(ui.IPStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightDB(fmt.Sprintf("%d", db.Port), widths[5]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightDB(db.Size, widths[6]) + " "
		sb.WriteString(ui.AZStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		sb.WriteString("\n")
	}

	sb.WriteString(ui.BorderStyle.Render(ui.BottomLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.BottomT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.BottomRight))
	sb.WriteString("\n")

	fmt.Print(sb.String())
	fmt.Printf("  %d databases\n", len(dbs))
}

func printDBDetails(db *types.Database) {
	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("Database Details"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  Name:      %s\n", ui.NameStyle.Render(db.Name))
	fmt.Printf("  Engine:    %s %s\n", db.Engine, db.Version)
	fmt.Printf("  State:     %s\n", formatDBStateText(db.State))
	fmt.Printf("  Endpoint:  %s:%d\n", db.Endpoint, db.Port)
	fmt.Printf("  Size:      %s\n", db.Size)
	if !db.CreatedAt.IsZero() {
		fmt.Printf("  Created:   %s\n", db.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  Provider:  %s\n", formatProviderName(db.Provider))
}

func formatDBState(state string, width int) string {
	var indicator string
	style := ui.StoppedStyle
	switch state {
	case "available":
		indicator = "●"
		style = ui.RunningStyle
	case "stopped":
		indicator = "○"
		style = ui.StoppedStyle
	case "creating", "modifying", "starting", "stopping", "rebooting", "backing-up":
		indicator = "◐"
		style = ui.PendingStyle
	default:
		indicator = "○"
	}
	cell := " " + padRightDB(indicator+" "+state, width) + " "
	return style.Render(cell)
}

func formatDBStateText(state string) string {
	switch state {
	case "available":
		return ui.RunningStyle.Render("● available")
	case "stopped":
		return ui.StoppedStyle.Render("○ stopped")
	default:
		return ui.PendingStyle.Render("◐ " + state)
	}
}

func padRightDB(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
