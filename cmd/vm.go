package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	gcpinternal "github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Manage virtual machines",
	Long: `Manage virtual machines across cloud providers.

Commands operate within the current context. Use 'cml use <context>' to switch.
Use --context flag to temporarily use a different context.

Examples:
  cml vm list                    # List running VMs
  cml vm list -s stopped         # List stopped VMs
  cml vm get web-01              # Get VM details
  cml vm connect web-01          # SSH/SSM to VM
  cml vm tunnel web-01 3306      # Port forward
  cml vm start web-01            # Start a VM
  cml vm stop web-01             # Stop a VM`,
}

var vmListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List virtual machines",
	Long: `List virtual machines in the current context.

Examples:
  cml vm list                    # List running VMs
  cml vm list -s stopped         # List stopped VMs
  cml vm list -s all             # List all VMs
  cml vm list --name web         # Filter by name
  cml vm list -t env=prod        # Filter by tag`,
	RunE: runVMList,
}

var vmGetCmd = &cobra.Command{
	Use:   "get <name-or-id>",
	Short: "Get VM details",
	Long: `Get detailed information about a specific VM.

Examples:
  cml vm get web-01
  cml vm get i-0abc123def456`,
	Args: cobra.ExactArgs(1),
	RunE: runVMGet,
}

var vmConnectCmd = &cobra.Command{
	Use:   "connect <name-or-id>",
	Short: "Connect to a VM",
	Long: `Establish an interactive session to a VM.

Uses AWS SSM for AWS instances and gcloud SSH for GCP instances.

Examples:
  cml vm connect web-01
  cml vm connect i-0abc123def456`,
	Args: cobra.ExactArgs(1),
	RunE: runVMConnect,
}

var vmTunnelCmd = &cobra.Command{
	Use:   "tunnel <name-or-id> <remote-port> [local-port]",
	Short: "Create a tunnel to a VM",
	Long: `Create a port forwarding tunnel to a VM.

If local-port is not specified, it defaults to remote-port.

Examples:
  cml vm tunnel web-01 3306           # Forward 3306:3306
  cml vm tunnel web-01 3306 13306     # Forward 13306:3306
  cml vm tunnel db-01 5432            # PostgreSQL tunnel`,
	Args: cobra.RangeArgs(2, 3),
	RunE: runVMTunnel,
}

var vmStartCmd = &cobra.Command{
	Use:   "start <name-or-id>",
	Short: "Start a VM",
	Long: `Start a stopped VM.

Examples:
  cml vm start web-01`,
	Args: cobra.ExactArgs(1),
	RunE: runVMStart,
}

var vmStopCmd = &cobra.Command{
	Use:   "stop <name-or-id>",
	Short: "Stop a VM",
	Long: `Stop a running VM.

Examples:
  cml vm stop web-01`,
	Args: cobra.ExactArgs(1),
	RunE: runVMStop,
}

var vmRebootCmd = &cobra.Command{
	Use:   "reboot <name-or-id>",
	Short: "Reboot a VM",
	Long: `Reboot a running VM.

Examples:
  cml vm reboot web-01`,
	Args: cobra.ExactArgs(1),
	RunE: runVMReboot,
}

var (
	vmListState       string
	vmListName        string
	vmListTags        []string
	vmListInteractive bool
	vmContextFlag     string
)

func init() {
	rootCmd.AddCommand(vmCmd)
	vmCmd.AddCommand(vmListCmd)
	vmCmd.AddCommand(vmGetCmd)
	vmCmd.AddCommand(vmConnectCmd)
	vmCmd.AddCommand(vmTunnelCmd)
	vmCmd.AddCommand(vmStartCmd)
	vmCmd.AddCommand(vmStopCmd)
	vmCmd.AddCommand(vmRebootCmd)

	// vm list flags
	vmListCmd.Flags().StringVarP(&vmListState, "state", "s", "", "Filter by state (running, stopped, all)")
	vmListCmd.Flags().StringVar(&vmListName, "name", "", "Filter by name pattern")
	vmListCmd.Flags().StringArrayVarP(&vmListTags, "tag", "t", nil, "Filter by tag (key=value)")
	vmListCmd.Flags().BoolVarP(&vmListInteractive, "interactive", "i", false, "Interactive selection mode")

	// Global context override
	vmCmd.PersistentFlags().StringVarP(&vmContextFlag, "context", "c", "", "Use specific context")
}

// getVMProvider returns the VM provider for the current or specified context
func getVMProvider(ctx context.Context) (provider.VMProvider, error) {
	// Get context (from flag or current)
	var ctxConfig *config.Context
	var ctxName string
	var err error

	if vmContextFlag != "" {
		// Use specified context
		cfg, loadErr := config.LoadCMLConfig()
		if loadErr != nil {
			return nil, loadErr
		}
		ctxConfig = cfg.Contexts[vmContextFlag]
		if ctxConfig == nil {
			return nil, fmt.Errorf("context %q not found", vmContextFlag)
		}
		ctxName = vmContextFlag
	} else {
		// Use current context
		ctxConfig, ctxName, err = config.GetCurrentContext()
		if err != nil {
			return nil, err
		}
		if ctxConfig == nil {
			return nil, fmt.Errorf("no context set. Use 'cml use <context>' to set one")
		}
	}

	// Create provider based on context
	switch ctxConfig.Provider {
	case "aws":
		client, err := aws.NewClient(ctx,
			aws.WithProfile(ctxConfig.Profile),
			aws.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create AWS client: %w", err)
		}
		return aws.NewVMProvider(client, ctxConfig.Profile, ctxConfig.Region), nil

	case "gcp":
		opts := []gcpinternal.Option{
			gcpinternal.WithProject(ctxConfig.Project),
			gcpinternal.WithRegion(ctxConfig.Region),
		}
		if ctxConfig.Bastion != "" {
			opts = append(opts,
				gcpinternal.WithBastion(ctxConfig.Bastion, ctxConfig.BastionZone),
				gcpinternal.WithBastionProject(ctxConfig.BastionProject),
				gcpinternal.WithBastionIAP(ctxConfig.BastionIAP),
			)
		}
		gcpClient, err := gcpinternal.NewClient(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCP client: %w", err)
		}
		return gcpinternal.NewVMProvider(gcpClient), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

func runVMList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	// Build filter
	filter := &provider.VMFilter{}

	if vmListState != "" && vmListState != "all" {
		filter.State = vmListState
	} else if vmListState == "" {
		filter.State = "running"
	}

	if vmListName != "" {
		filter.Name = vmListName
	}

	// Parse tags
	if len(vmListTags) > 0 {
		filter.Tags = make(map[string]string)
		for _, t := range vmListTags {
			parts := strings.SplitN(t, "=", 2)
			if len(parts) == 2 {
				filter.Tags[parts[0]] = parts[1]
			}
		}
	}

	// List VMs
	vms, err := vmProvider.List(ctx, filter)
	if err != nil {
		return err
	}

	if len(vms) == 0 {
		fmt.Println("No VMs found")
		return nil
	}

	if vmListInteractive {
		vm, action, err := ui.SelectVM(vms)
		if err != nil {
			return nil // cancelled — silent exit
		}
		switch action {
		case ui.VMActionConnect:
			fmt.Printf("Connecting to %s...\n", vm.Name)
			return vmProvider.Connect(ctx, vm.ID)
		case ui.VMActionStart:
			if err := vmProvider.Start(ctx, vm.ID); err != nil {
				return err
			}
			fmt.Printf("Starting VM: %s\n", vm.Name)
		case ui.VMActionStop:
			if err := vmProvider.Stop(ctx, vm.ID); err != nil {
				return err
			}
			fmt.Printf("Stopping VM: %s\n", vm.Name)
		}
		return nil
	}

	// Print table
	printVMTable(vms)

	return nil
}

func runVMGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	vm, err := vmProvider.Get(ctx, args[0])
	if err != nil {
		return err
	}

	// Print VM details
	printVMDetails(vm)

	return nil
}

func runVMConnect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Connecting to %s...\n", args[0])
	return vmProvider.Connect(ctx, args[0])
}

func runVMTunnel(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	// Parse ports
	remotePort := 0
	localPort := 0

	_, err = fmt.Sscanf(args[1], "%d", &remotePort)
	if err != nil {
		return fmt.Errorf("invalid remote port: %s", args[1])
	}

	if len(args) > 2 {
		_, err = fmt.Sscanf(args[2], "%d", &localPort)
		if err != nil {
			return fmt.Errorf("invalid local port: %s", args[2])
		}
	} else {
		localPort = remotePort
	}

	opts := &provider.TunnelOptions{
		LocalPort:  localPort,
		RemotePort: remotePort,
	}

	fmt.Printf("Creating tunnel to %s: localhost:%d -> remote:%d\n", args[0], localPort, remotePort)
	fmt.Println("Press Ctrl+C to close the tunnel")

	return vmProvider.Tunnel(ctx, args[0], opts)
}

func runVMStart(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	if err := vmProvider.Start(ctx, args[0]); err != nil {
		return err
	}

	fmt.Printf("Starting VM: %s\n", args[0])
	return nil
}

func runVMStop(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	if err := vmProvider.Stop(ctx, args[0]); err != nil {
		return err
	}

	fmt.Printf("Stopping VM: %s\n", args[0])
	return nil
}

func runVMReboot(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	vmProvider, err := getVMProvider(ctx)
	if err != nil {
		return err
	}

	if err := vmProvider.Reboot(ctx, args[0]); err != nil {
		return err
	}

	fmt.Printf("Rebooting VM: %s\n", args[0])
	return nil
}

// printVMTable prints VMs in a table format
func printVMTable(vms []types.VM) {
	headers := []string{"ID", "Name", "Private IP", "State", "Type", "Zone"}
	widths := []int{22, 30, 15, 10, 14, 18}

	var sb strings.Builder

	// Top border
	sb.WriteString(ui.BorderStyle.Render(ui.TopLeft))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.TopT))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.TopRight))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	for i, h := range headers {
		cell := " " + padRightVM(h, widths[i]) + " "
		sb.WriteString(ui.HeaderStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
	}
	sb.WriteString("\n")

	// Header separator
	sb.WriteString(ui.BorderStyle.Render(ui.LeftT))
	for i, w := range widths {
		sb.WriteString(ui.BorderStyle.Render(strings.Repeat(ui.Horizontal, w+2)))
		if i < len(widths)-1 {
			sb.WriteString(ui.BorderStyle.Render(ui.Cross))
		}
	}
	sb.WriteString(ui.BorderStyle.Render(ui.RightT))
	sb.WriteString("\n")

	// Data rows
	for _, vm := range vms {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// ID
		cell := " " + padRightVM(vm.ID, widths[0]) + " "
		sb.WriteString(ui.IDStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Name
		cell = " " + padRightVM(vm.Name, widths[1]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Private IP
		cell = " " + padRightVM(vm.PrivateIP, widths[2]) + " "
		sb.WriteString(ui.IPStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// State
		stateCell := formatVMState(string(vm.State), widths[3])
		sb.WriteString(stateCell)
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Type
		cell = " " + padRightVM(vm.Type, widths[4]) + " "
		sb.WriteString(ui.TypeStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		// Zone
		cell = " " + padRightVM(vm.Zone, widths[5]) + " "
		sb.WriteString(ui.AZStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		sb.WriteString("\n")
	}

	// Bottom border
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

	// Summary
	fmt.Printf("  %d VMs\n", len(vms))
}

func printVMDetails(vm *types.VM) {
	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("VM Details"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  ID:         %s\n", ui.IDStyle.Render(vm.ID))
	fmt.Printf("  Name:       %s\n", ui.NameStyle.Render(vm.Name))
	fmt.Printf("  State:      %s\n", formatVMStateText(string(vm.State)))
	fmt.Printf("  Type:       %s\n", vm.Type)
	fmt.Printf("  Zone:       %s\n", vm.Zone)
	fmt.Printf("  Private IP: %s\n", vm.PrivateIP)
	if vm.PublicIP != "" {
		fmt.Printf("  Public IP:  %s\n", vm.PublicIP)
	}
	if vm.ASG != "" {
		label := "ASG"
		if vm.Provider == "gcp" {
			label = "IG "
		}
		fmt.Printf("  %s:        %s\n", label, vm.ASG)
	}
	fmt.Printf("  Launched:   %s\n", vm.LaunchedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Provider:   %s\n", formatProviderName(vm.Provider))

	if len(vm.Tags) > 0 {
		fmt.Println()
		fmt.Println(ui.MutedStyle.Render("  Tags:"))
		for k, v := range vm.Tags {
			if k != "Name" {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}
}

func formatVMState(state string, width int) string {
	var indicator string
	var style = ui.StoppedStyle

	switch state {
	case "running":
		indicator = "●"
		style = ui.RunningStyle
	case "stopped":
		indicator = "○"
		style = ui.StoppedStyle
	case "pending", "stopping":
		indicator = "◐"
		style = ui.PendingStyle
	default:
		indicator = "○"
		style = ui.StoppedStyle
	}

	stateText := indicator + " " + state
	cell := " " + padRightVM(stateText, width) + " "
	return style.Render(cell)
}

func formatVMStateText(state string) string {
	switch state {
	case "running":
		return ui.RunningStyle.Render("● running")
	case "stopped":
		return ui.StoppedStyle.Render("○ stopped")
	case "pending", "stopping":
		return ui.PendingStyle.Render("◐ " + state)
	default:
		return state
	}
}

func formatProviderName(provider string) string {
	switch provider {
	case "aws":
		return ui.AWSStyle.Render("AWS")
	case "gcp":
		return ui.GCPStyle.Render("GCP")
	default:
		return provider
	}
}

func padRightVM(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
