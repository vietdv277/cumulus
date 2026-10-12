package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var storageCmd = &cobra.Command{
	Use:     "storage",
	Aliases: []string{"s3"},
	Short:   "Manage object storage (S3 / GCS)",
	Long: `Manage object storage within the current context.

Paths use s3://bucket/key on AWS contexts and gs://bucket/key on GCP contexts.
Local paths are anything else.

Examples:
  cml storage ls                              # List buckets
  cml storage ls s3://my-bucket               # List objects in bucket (AWS)
  cml storage ls gs://my-bucket logs/         # List objects under prefix (GCP)
  cml storage cp file.txt s3://b/key.txt      # Upload
  cml storage cp s3://b/key.txt ./            # Download
  cml storage cp s3://b/a gs://b/b            # Copy (one side must be remote)
  cml storage sync ./dir s3://b/dir --delete  # Sync directory
  cml storage presign gs://b/key.txt          # Presigned GET URL`,
}

var storageLsCmd = &cobra.Command{
	Use:     "ls [s3://bucket|gs://bucket [prefix]]",
	Aliases: []string{"list"},
	Short:   "List buckets, or objects under a bucket/prefix",
	Args:    cobra.RangeArgs(0, 2),
	RunE:    runStorageLs,
}

var storageCpCmd = &cobra.Command{
	Use:   "cp <src> <dst>",
	Short: "Copy a single object between local and remote (s3/gs)",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageCp,
}

var storageSyncCmd = &cobra.Command{
	Use:   "sync <src> <dst>",
	Short: "Sync directories (delegates to aws s3 sync / gcloud storage rsync)",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageSync,
}

var storagePresignCmd = &cobra.Command{
	Use:   "presign <s3://bucket/key|gs://bucket/key>",
	Short: "Generate a presigned GET URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runStoragePresign,
}

var (
	storageContextFlag string
	storageSyncDelete  bool
	storageSyncDryRun  bool
	storageSyncYes     bool
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
	storageSyncCmd.Flags().BoolVarP(&storageSyncYes, "yes", "y", false, "Skip confirmation prompt")

	storagePresignCmd.Flags().IntVar(&storagePresignTTL, "expires", 3600, "URL TTL in seconds")

	storageCmd.PersistentFlags().StringVarP(&storageContextFlag, "context", "c", "", "Use specific context")
}

func getStorageProvider(ctx context.Context) (provider.StorageProvider, string, error) {
	ctxConfig, ctxName, err := resolveContext(storageContextFlag)
	if err != nil {
		return nil, "", err
	}

	switch ctxConfig.Provider {
	case "aws":
		client, err := aws.NewClient(ctx,
			aws.WithProfile(ctxConfig.Profile),
			aws.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create AWS client: %w", err)
		}
		return aws.NewStorageProvider(client, ctxConfig.Profile, ctxConfig.Region), "aws", nil

	case "gcp":
		client, err := gcp.NewClient(ctx,
			gcp.WithProject(ctxConfig.Project),
			gcp.WithRegion(ctxConfig.Region),
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create GCP client: %w", err)
		}
		return gcp.NewStorageProvider(client), "gcp", nil

	default:
		return nil, "", fmt.Errorf("unknown provider: %s (context: %s)", ctxConfig.Provider, ctxName)
	}
}

// storageRemoteSchemes maps each provider to the URI scheme its storage
// paths use.
var storageRemoteSchemes = map[string]string{
	"aws": "s3://",
	"gcp": "gs://",
}

// storageScheme returns the path scheme for the context's provider
// (s3:// unless the provider is gcp).
func storageScheme(providerName string) string {
	if s := storageRemoteSchemes[providerName]; s != "" {
		return s
	}
	return "s3://"
}

// isRemoteSchemePath reports whether p starts with any storage scheme, to
// detect a scheme/provider mismatch.
func isRemoteSchemePath(p string) bool {
	for _, scheme := range storageRemoteSchemes {
		if strings.HasPrefix(p, scheme) {
			return true
		}
	}
	return false
}

func runStorageLs(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	sp, providerName, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}
	scheme := storageScheme(providerName)

	if len(args) == 0 {
		buckets, err := sp.ListBuckets(ctx)
		if err != nil {
			return err
		}
		if len(buckets) == 0 && out == OutputTable {
			fmt.Println("No buckets found")
			return nil
		}
		return render(os.Stdout, out, buckets, func() { printBucketTable(buckets) })
	}

	bucket, prefix, err := parseLsTarget(args, scheme)
	if err != nil {
		return err
	}

	objects, err := sp.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return err
	}
	if len(objects) == 0 && out == OutputTable {
		fmt.Println("No objects found")
		return nil
	}
	// The bucket header is info, not the document — keep stdout clean in
	// json/yaml mode.
	if out != OutputTable {
		fmt.Fprintf(os.Stderr, "%s%s\n", scheme, bucket)
	}
	return render(os.Stdout, out, objects, func() { printObjectTable(scheme+bucket, objects) })
}

func runStorageCp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, _, err := getStorageProvider(ctx)
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
	sp, _, err := getStorageProvider(ctx)
	if err != nil {
		return err
	}
	// --dry-run deletes nothing, so it needs no confirmation.
	if storageSyncDelete && !storageSyncDryRun {
		proceed, err := confirmAction(storageSyncYes, os.Stdin, os.Stdout,
			fmt.Sprintf("Sync %s -> %s in context %s and delete destination objects not in src? [y/N]",
				args[0], args[1], contextNameForPrompt(storageContextFlag)),
			fmt.Sprintf("refusing to sync %s with --delete without confirmation; pass --yes", args[1]),
		)
		if err != nil {
			return err
		}
		if !proceed {
			fmt.Println("Sync cancelled")
			return nil
		}
	}
	opts := &provider.SyncOptions{Delete: storageSyncDelete, DryRun: storageSyncDryRun}
	return sp.Sync(ctx, args[0], args[1], opts)
}

func runStoragePresign(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sp, _, err := getStorageProvider(ctx)
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

// parseLsTarget accepts forms: ["<scheme://bucket>"],
// ["<scheme://bucket/prefix>"], or ["<scheme://bucket>", "prefix"] and returns
// (bucket, prefix). A path under a different remote scheme is a mismatch and
// errors naming the expected scheme.
func parseLsTarget(args []string, scheme string) (string, string, error) {
	first := args[0]
	if !strings.HasPrefix(first, scheme) {
		if isRemoteSchemePath(first) {
			return "", "", fmt.Errorf("this context uses %s paths, got %q", scheme, first)
		}
		return "", "", fmt.Errorf("bucket must be a %s URI, got %q", scheme, first)
	}
	rest := strings.TrimPrefix(first, scheme)
	parts := strings.SplitN(rest, "/", 2)
	bucket := parts[0]
	prefix := ""
	if len(parts) == 2 {
		prefix = parts[1]
	}
	if bucket == "" {
		return "", "", fmt.Errorf("invalid %s URI: %s", scheme, first)
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

func printObjectTable(bucketURI string, objects []types.Object) {
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
	fmt.Printf("  %s\n", bucketURI)
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
