package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	gcpinternal "github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/kubeconfig"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var k8sCmd = &cobra.Command{
	Use:   "k8s",
	Short: "Manage Kubernetes clusters",
	Long: `Manage Kubernetes clusters across cloud providers.

Commands operate within the current context. Use 'cml use <context>' to switch.
Use --context flag to temporarily use a different context.

Examples:
  cml k8s list                    # List clusters
  cml k8s get my-cluster          # Cluster details
  cml k8s use my-cluster          # Update kubeconfig and switch kubectl context
  cml k8s contexts                # List kubectl contexts`,
}

var k8sListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List Kubernetes clusters",
	RunE:    runK8sList,
}

var k8sGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get cluster details",
	Args:  cobra.ExactArgs(1),
	RunE:  runK8sGet,
}

var k8sUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Update kubeconfig for a cluster",
	Long: `Update ~/.kube/config with credentials for a cluster and switch the
kubectl current-context to it. Shells out to 'aws eks update-kubeconfig'
(AWS) or 'gcloud container clusters get-credentials' (GCP).`,
	Args: cobra.ExactArgs(1),
	RunE: runK8sUse,
}

var k8sContextsCmd = &cobra.Command{
	Use:   "contexts",
	Short: "List kubectl contexts from ~/.kube/config",
	RunE:  runK8sContexts,
}

var k8sConnectCmd = &cobra.Command{
	Use:   "connect <cluster>",
	Short: "Open an SSM tunnel to the context bastion and launch a subshell with HTTPS_PROXY set",
	Long: `Open an SSM port-forwarding session to the AWS context's bastion instance and
drop into an interactive subshell with HTTPS_PROXY pointing at the forwarded port.
kubectl run inside the subshell reaches a private EKS API through the bastion.
Exiting the subshell tears the tunnel down.

Requires the active AWS context to have a 'bastion' field set (EC2 instance ID).
The remote port defaults to 8888 and can be overridden via 'bastion_port' on the
context or --local-port on the command.

Examples:
  cml k8s connect prod-cluster
  cml k8s connect prod-cluster --bastion i-013xxxxx --local-port 9999`,
	Args: cobra.ExactArgs(1),
	RunE: runK8sConnect,
}

var (
	k8sListName         string
	k8sListInteractive  bool
	k8sContextFlag      string
	k8sConnectBastion   string
	k8sConnectLocalPort int
)

func init() {
	rootCmd.AddCommand(k8sCmd)
	k8sCmd.AddCommand(k8sListCmd)
	k8sCmd.AddCommand(k8sGetCmd)
	k8sCmd.AddCommand(k8sUseCmd)
	k8sCmd.AddCommand(k8sContextsCmd)
	k8sCmd.AddCommand(k8sConnectCmd)

	k8sListCmd.Flags().StringVar(&k8sListName, "name", "", "Filter by name substring")
	k8sListCmd.Flags().BoolVarP(&k8sListInteractive, "interactive", "i", false, "Interactive selection mode")

	k8sConnectCmd.Flags().StringVar(&k8sConnectBastion, "bastion", "", "Override context bastion instance ID")
	k8sConnectCmd.Flags().IntVar(&k8sConnectLocalPort, "local-port", 0, "Local port for HTTPS_PROXY (default: bastion_port or 8888)")

	k8sCmd.PersistentFlags().StringVarP(&k8sContextFlag, "context", "c", "", "Use specific context")
}

func getK8sProvider(ctx context.Context) (provider.K8sProvider, error) {
	var ctxConfig *config.Context
	var ctxName string
	var err error

	if k8sContextFlag != "" {
		cfg, loadErr := config.LoadCMLConfig()
		if loadErr != nil {
			return nil, loadErr
		}
		ctxConfig = cfg.Contexts[k8sContextFlag]
		if ctxConfig == nil {
			return nil, fmt.Errorf("context %q not found", k8sContextFlag)
		}
		ctxName = k8sContextFlag
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
		return aws.NewK8sProvider(client, ctxConfig.Profile, ctxConfig.Region), nil

	case "gcp":
		gcpClient, err := gcpinternal.NewClient(ctx,
			gcpinternal.WithProject(ctxConfig.Project),
			gcpinternal.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCP client: %w", err)
		}
		return gcpinternal.NewK8sProvider(gcpClient), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

func runK8sList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	p, err := getK8sProvider(ctx)
	if err != nil {
		return err
	}

	clusters, err := p.ListClusters(ctx)
	if err != nil {
		return err
	}

	if k8sListName != "" {
		q := strings.ToLower(k8sListName)
		filtered := clusters[:0]
		for _, c := range clusters {
			if strings.Contains(strings.ToLower(c.Name), q) {
				filtered = append(filtered, c)
			}
		}
		clusters = filtered
	}

	if len(clusters) == 0 {
		fmt.Println("No clusters found")
		return nil
	}

	if k8sListInteractive {
		selected, err := ui.SelectK8sCluster(clusters)
		if err != nil {
			return nil
		}
		fmt.Printf("Updating kubeconfig for %s...\n", selected.Name)
		return p.UpdateKubeconfig(ctx, selected.Name)
	}

	printK8sTable(clusters)
	return nil
}

func runK8sGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	p, err := getK8sProvider(ctx)
	if err != nil {
		return err
	}

	c, err := p.GetCluster(ctx, args[0])
	if err != nil {
		return err
	}

	printK8sDetails(c)
	return nil
}

func runK8sUse(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	p, err := getK8sProvider(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Updating kubeconfig for %s...\n", args[0])
	return p.UpdateKubeconfig(ctx, args[0])
}

func runK8sContexts(cmd *cobra.Command, args []string) error {
	contexts, current, err := kubeconfig.LoadContexts()
	if err != nil {
		return err
	}
	if len(contexts) == 0 {
		fmt.Println("No kubectl contexts found")
		return nil
	}

	nameW, clusterW, userW := len("NAME"), len("CLUSTER"), len("USER")
	for _, c := range contexts {
		if len(c.Name) > nameW {
			nameW = len(c.Name)
		}
		if len(c.Cluster) > clusterW {
			clusterW = len(c.Cluster)
		}
		if len(c.User) > userW {
			userW = len(c.User)
		}
	}

	fmt.Printf("  %s  %s  %s\n",
		padRightVM("NAME", nameW),
		padRightVM("CLUSTER", clusterW),
		padRightVM("USER", userW))
	for _, c := range contexts {
		marker := " "
		if c.Name == current {
			marker = "*"
		}
		fmt.Printf("%s %s  %s  %s\n",
			marker,
			padRightVM(c.Name, nameW),
			padRightVM(c.Cluster, clusterW),
			padRightVM(c.User, userW))
	}
	return nil
}

// resolveContextForK8s returns the context config and name used by the k8s commands,
// respecting --context when set and falling back to the current context otherwise.
func resolveContextForK8s() (*config.Context, string, error) {
	if k8sContextFlag != "" {
		cfg, err := config.LoadCMLConfig()
		if err != nil {
			return nil, "", err
		}
		ctxConfig := cfg.Contexts[k8sContextFlag]
		if ctxConfig == nil {
			return nil, "", fmt.Errorf("context %q not found", k8sContextFlag)
		}
		return ctxConfig, k8sContextFlag, nil
	}
	ctxConfig, ctxName, err := config.GetCurrentContext()
	if err != nil {
		return nil, "", err
	}
	if ctxConfig == nil {
		return nil, "", fmt.Errorf("no context set. Use 'cml use <context>' to set one")
	}
	return ctxConfig, ctxName, nil
}

func runK8sConnect(cmd *cobra.Command, args []string) error {
	cluster := args[0]

	ctxConfig, ctxName, err := resolveContextForK8s()
	if err != nil {
		return err
	}
	if ctxConfig.Provider != "aws" {
		return fmt.Errorf("k8s connect is AWS-only for now (context %q is %s)", ctxName, ctxConfig.Provider)
	}

	bastion := k8sConnectBastion
	if bastion == "" {
		bastion = ctxConfig.Bastion
	}
	if bastion == "" {
		return fmt.Errorf("no bastion configured for context %q — set one with:\n"+
			"  cml use update %s --bastion i-xxxxxx [--bastion-port 8888]", ctxName, ctxName)
	}

	remotePort := ctxConfig.BastionPort
	if remotePort == 0 {
		remotePort = 8888
	}
	localPort := k8sConnectLocalPort
	if localPort == 0 {
		localPort = remotePort
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := aws.NewClient(ctx,
		aws.WithProfile(ctxConfig.Profile),
		aws.WithRegion(ctxConfig.Region),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}
	vmProvider := aws.NewVMProvider(client, ctxConfig.Profile, ctxConfig.Region)

	fmt.Fprintf(os.Stderr, "→ Starting SSM tunnel to %s:%d (local :%d)\n", bastion, remotePort, localPort)
	tunnelCmd, err := vmProvider.StartPortForward(ctx, bastion, remotePort, localPort)
	if err != nil {
		return err
	}
	defer func() {
		if tunnelCmd.Process != nil {
			_ = tunnelCmd.Process.Signal(syscall.SIGTERM)
			// Small grace period, then force kill.
			done := make(chan struct{})
			go func() { _ = tunnelCmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = tunnelCmd.Process.Kill()
				<-done
			}
		}
	}()

	if err := waitForPort(ctx, localPort, 10*time.Second); err != nil {
		return fmt.Errorf("tunnel did not become ready: %w", err)
	}

	proxy := fmt.Sprintf("http://localhost:%d", localPort)
	fmt.Fprintf(os.Stderr, "→ Proxy ready at %s\n", proxy)
	fmt.Fprintln(os.Stderr, "→ Entering subshell (HTTPS_PROXY set). Type 'exit' to disconnect.")
	fmt.Fprintln(os.Stderr)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	sub := exec.Command(shell, "-i")
	sub.Env = append(os.Environ(),
		"HTTPS_PROXY="+proxy,
		"https_proxy="+proxy,
		"HTTP_PROXY="+proxy,
		"http_proxy="+proxy,
		"CML_K8S_CONNECT="+cluster,
		"CML_CONTEXT="+ctxName,
	)
	sub.Stdin = os.Stdin
	sub.Stdout = os.Stdout
	sub.Stderr = os.Stderr

	// Forward SIGINT to the subshell rather than letting it kill us directly;
	// we want to clean up the tunnel in the deferred handler either way.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			if sub.Process != nil {
				_ = sub.Process.Signal(sig)
			}
		}
	}()

	if err := sub.Run(); err != nil {
		// Non-zero exit from the subshell (e.g. user ran a command that failed)
		// isn't a cml error; only surface startup failures.
		if _, ok := err.(*exec.ExitError); !ok {
			return fmt.Errorf("subshell: %w", err)
		}
	}

	fmt.Fprintln(os.Stderr, "→ Tunnel closed.")
	return nil
}

// waitForPort polls 127.0.0.1:<port> until a TCP connection succeeds or the timeout expires.
func waitForPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s: %w", addr, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func printK8sTable(clusters []types.K8sCluster) {
	headers := []string{"Name", "Version", "Status", "Region", "Provider"}
	widths := []int{30, 12, 14, 18, 10}

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
		cell := " " + padRightVM(h, widths[i]) + " "
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

	for _, c := range clusters {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell := " " + padRightVM(c.Name, widths[0]) + " "
		sb.WriteString(ui.NameStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightVM(c.Version, widths[1]) + " "
		sb.WriteString(ui.MutedStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightVM(c.Status, widths[2]) + " "
		sb.WriteString(k8sStatusCell(c.Status, cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightVM(c.Region, widths[3]) + " "
		sb.WriteString(ui.AZStyle.Render(cell))
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))

		cell = " " + padRightVM(strings.ToUpper(c.Provider), widths[4]) + " "
		sb.WriteString(k8sProviderCell(c.Provider, cell))
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
	fmt.Printf("  %d clusters\n", len(clusters))
}

func printK8sDetails(c *types.K8sCluster) {
	fmt.Println()
	fmt.Println(ui.HeaderStyle.Render("Cluster Details"))
	fmt.Println(ui.MutedStyle.Render("───────────────────────────────"))
	fmt.Printf("  Name:      %s\n", ui.NameStyle.Render(c.Name))
	fmt.Printf("  Version:   %s\n", c.Version)
	fmt.Printf("  Status:    %s\n", k8sStatusText(c.Status))
	fmt.Printf("  Region:    %s\n", c.Region)
	fmt.Printf("  Endpoint:  %s\n", c.Endpoint)
	if c.NodeCount > 0 {
		fmt.Printf("  Nodes:     %d\n", c.NodeCount)
	}
	if !c.CreatedAt.IsZero() {
		fmt.Printf("  Created:   %s\n", c.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  Provider:  %s\n", formatProviderName(c.Provider))
}

func k8sStatusCell(status, cell string) string {
	switch strings.ToUpper(status) {
	case "ACTIVE", "RUNNING":
		return ui.RunningStyle.Render(cell)
	case "CREATING", "UPDATING", "PROVISIONING", "RECONCILING":
		return ui.PendingStyle.Render(cell)
	default:
		return ui.StoppedStyle.Render(cell)
	}
}

func k8sProviderCell(provider, cell string) string {
	switch provider {
	case "aws":
		return ui.AWSStyle.Render(cell)
	case "gcp":
		return ui.GCPStyle.Render(cell)
	default:
		return ui.MutedStyle.Render(cell)
	}
}

func k8sStatusText(status string) string {
	switch strings.ToUpper(status) {
	case "ACTIVE", "RUNNING":
		return ui.RunningStyle.Render("● " + status)
	case "CREATING", "UPDATING", "PROVISIONING", "RECONCILING":
		return ui.PendingStyle.Render("◐ " + status)
	default:
		return ui.StoppedStyle.Render("○ " + status)
	}
}
