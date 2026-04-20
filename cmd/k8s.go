package cmd

import (
	"context"
	"fmt"
	"strings"

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

var (
	k8sListName        string
	k8sListInteractive bool
	k8sContextFlag     string
)

func init() {
	rootCmd.AddCommand(k8sCmd)
	k8sCmd.AddCommand(k8sListCmd)
	k8sCmd.AddCommand(k8sGetCmd)
	k8sCmd.AddCommand(k8sUseCmd)
	k8sCmd.AddCommand(k8sContextsCmd)

	k8sListCmd.Flags().StringVar(&k8sListName, "name", "", "Filter by name substring")
	k8sListCmd.Flags().BoolVarP(&k8sListInteractive, "interactive", "i", false, "Interactive selection mode")

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
