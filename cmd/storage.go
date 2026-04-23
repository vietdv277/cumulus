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

var storageCmd = &cobra.Command{
	Use:     "storage",
	Aliases: []string{"s3"},
	Short:   "Manage object storage (S3 / GCS)",
	Long: `Manage object storage within the current context.

Paths use the s3://bucket/key syntax. Local paths are anything else.

Examples:
  cml storage ls                              # List buckets
  cml storage ls s3://my-bucket               # List objects in bucket
  cml storage ls s3://my-bucket logs/         # List objects under prefix
  cml storage cp file.txt s3://b/key.txt      # Upload
  cml storage cp s3://b/key.txt ./            # Download
  cml storage sync ./dir s3://b/dir --delete  # Sync directory
  cml storage presign s3://b/key.txt          # Presigned GET URL`,
}

var storageLsCmd = &cobra.Command{
	Use:     "ls [s3://bucket [prefix]]",
	Aliases: []string{"list"},
	Short:   "List buckets, or objects under a bucket/prefix",
	Args:    cobra.RangeArgs(0, 2),
	RunE:    runStorageLs,
}

var storageCpCmd = &cobra.Command{
	Use:   "cp <src> <dst>",
	Short: "Copy a single object between local and S3",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageCp,
}

var storageSyncCmd = &cobra.Command{
	Use:   "sync <src> <dst>",
	Short: "Sync directories (delegates to aws s3 sync)",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageSync,
}

var storagePresignCmd = &cobra.Command{
	Use:   "presign <s3://bucket/key>",
	Short: "Generate a presigned GET URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runStoragePresign,
}

var (
	storageContextFlag string
	storageSyncDelete  bool
	storageSyncDryRun  bool
	storagePresignTTL  int
)

func init() {
	rootCmd.AddCommand(storageCmd)
	storageCmd.AddCommand(storageLsCmd)
	storageCmd.AddCommand(storageCpCmd)
	storageCmd.AddCommand(storageSyncCmd)
	storageCmd.AddCommand(storagePresignCmd)

	storageSyncCmd.Flags().BoolVar(&storageSyncDelete, "delete", false, "Delete files in dst not present in src")
	storageSyncCmd.Flags().BoolVar(&storageSyncDryRun, "dry-run", false, "Show what would be transferred")

	storagePresignCmd.Flags().IntVar(&storagePresignTTL, "expires", 3600, "URL TTL in seconds")

	storageCmd.PersistentFlags().StringVarP(&storageContextFlag, "context", "c", "", "Use specific context")
}

func getStorageProvider(ctx context.Context) (provider.StorageProvider, error) {
	var ctxConfig *config.Context
	var ctxName string
	var err error

	if storageContextFlag != "" {
		cfg, loadErr := config.LoadCMLConfig()
		if loadErr != nil {
			return nil, loadErr
		}
		ctxConfig = cfg.Contexts[storageContextFlag]
		if ctxConfig == nil {
			return nil, fmt.Errorf("context %q not found", storageContextFlag)
		}
		ctxName = storageContextFlag
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
		return aws.NewStorageProvider(client, ctxConfig.Profile, ctxConfig.Region), nil

	case "gcp":
		return nil, fmt.Errorf("storage commands are not yet implemented for GCP (context: %s)", ctxName)

	default:
		return nil, fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

func runStorageLs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		buckets, err := sp.ListBuckets(ctx)
		if err != nil {
			return err
		}
		if len(buckets) == 0 {
			fmt.Println("No buckets found")
			return nil
		}
		printBucketTable(buckets)
		return nil
	}

	bucket, prefix, err := parseLsTarget(args)
	if err != nil {
		return err
	}

	objects, err := sp.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		fmt.Println("No objects found")
		return nil
	}
	printObjectTable(bucket, objects)
	return nil
}

func runStorageCp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}
	if err := sp.Copy(ctx, args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("Copied %s -> %s\n", args[0], args[1])
	return nil
}

func runStorageSync(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}
	opts := &provider.SyncOptions{Delete: storageSyncDelete, DryRun: storageSyncDryRun}
	return sp.Sync(ctx, args[0], args[1], opts)
}

func runStoragePresign(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}
	url, err := sp.Presign(ctx, args[0], storagePresignTTL)
	if err != nil {
		return err
	}
	fmt.Println(url)
	return nil
}

// parseLsTarget accepts forms: ["s3://bucket"], ["s3://bucket/prefix"],
// or ["s3://bucket", "prefix"] and returns (bucket, prefix).
func parseLsTarget(args []string) (string, string, error) {
	first := args[0]
	if !strings.HasPrefix(first, "s3://") {
		return "", "", fmt.Errorf("bucket must be an s3:// URI, got %q", first)
	}
	rest := strings.TrimPrefix(first, "s3://")
	parts := strings.SplitN(rest, "/", 2)
	bucket := parts[0]
	prefix := ""
	if len(parts) == 2 {
		prefix = parts[1]
	}
	if bucket == "" {
		return "", "", fmt.Errorf("invalid s3 URI: %s", first)
	}
	if len(args) == 2 {
		if prefix != "" {
			return "", "", fmt.Errorf("prefix specified twice: in URI and as positional arg")
		}
		prefix = args[1]
	}
	return bucket, prefix, nil
}

func printBucketTable(buckets []types.Bucket) {
	headers := []string{"Name", "Created", "Provider"}
	widths := []int{50, 20, 10}
	rows := make([][]string, 0, len(buckets))
	for _, b := range buckets {
		created := ""
		if !b.CreatedAt.IsZero() {
			created = b.CreatedAt.Format("2006-01-02 15:04:05")
		}
		rows = append(rows, []string{b.Name, created, b.Provider})
	}
	renderSimpleTable(headers, widths, rows)
	fmt.Printf("  %d buckets\n", len(buckets))
}

func printObjectTable(bucket string, objects []types.Object) {
	headers := []string{"Key", "Size", "Last Modified", "Storage Class"}
	widths := []int{60, 12, 20, 14}
	rows := make([][]string, 0, len(objects))
	for _, o := range objects {
		modified := ""
		if !o.LastModified.IsZero() {
			modified = o.LastModified.Format("2006-01-02 15:04:05")
		}
		class := o.StorageClass
		if class == "" {
			class = "STANDARD"
		}
		rows = append(rows, []string{o.Key, humanSize(o.Size), modified, class})
	}
	fmt.Printf("  s3://%s\n", bucket)
	renderSimpleTable(headers, widths, rows)
	fmt.Printf("  %d objects\n", len(objects))
}

func renderSimpleTable(headers []string, widths []int, rows [][]string) {
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
		cell := " " + padRightStorage(h, widths[i]) + " "
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

	for _, row := range rows {
		sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
		for i, val := range row {
			cell := " " + padRightStorage(val, widths[i]) + " "
			sb.WriteString(cell)
			sb.WriteString(ui.BorderStyle.Render(ui.Vertical))
		}
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
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"K", "M", "G", "T", "P"}[exp]
	return fmt.Sprintf("%.1f %sB", float64(n)/float64(div), suffix)
}

func padRightStorage(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", width-sw)
}
