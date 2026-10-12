package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// GCPStorageProvider implements provider.StorageProvider for Google Cloud Storage.
type GCPStorageProvider struct {
	client *Client
	// endpoint overrides the GCS API endpoint (used by tests).
	endpoint string
}

// NewStorageProvider creates a new GCS storage provider backed by the given Client.
func NewStorageProvider(client *Client) *GCPStorageProvider {
	return &GCPStorageProvider{client: client}
}

// errPresignNeedsSA is returned when ADC credentials cannot sign locally:
// only service-account keys carry an RSA private key.
var errPresignNeedsSA = errors.New("presign needs service-account credentials (key file or impersonation)")

// newStorageService builds a GCS JSON API client from the shared credentials.
func (p *GCPStorageProvider) newStorageService(ctx context.Context) (*storage.Service, error) {
	opts := []option.ClientOption{option.WithTokenSource(p.client.Credentials().TokenSource)}
	if p.endpoint != "" {
		opts = append(opts, option.WithEndpoint(p.endpoint))
	}
	return storage.NewService(ctx, opts...)
}

// ListBuckets returns the buckets of the context's project.
func (p *GCPStorageProvider) ListBuckets(ctx context.Context) ([]types.Bucket, error) {
	svc, err := p.newStorageService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	buckets := make([]types.Bucket, 0)
	req := svc.Buckets.List(p.client.Project())
	err = req.Pages(ctx, func(page *storage.Buckets) error {
		for _, b := range page.Items {
			bucket := types.Bucket{Name: b.Name, Provider: "gcp", Region: b.Location}
			if b.TimeCreated != "" {
				if created, err := time.Parse(time.RFC3339, b.TimeCreated); err == nil {
					bucket.CreatedAt = created
				}
			}
			buckets = append(buckets, bucket)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}
	return buckets, nil
}

// ListObjects returns objects under prefix in bucket.
func (p *GCPStorageProvider) ListObjects(ctx context.Context, bucket, prefix string) ([]types.Object, error) {
	svc, err := p.newStorageService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	var objects []types.Object
	req := svc.Objects.List(bucket)
	if prefix != "" {
		req.Prefix(prefix)
	}
	err = req.Pages(ctx, func(page *storage.Objects) error {
		for _, o := range page.Items {
			obj := types.Object{
				Key:          o.Name,
				ETag:         strings.Trim(o.Etag, `"`),
				StorageClass: o.StorageClass,
				Size:         int64(o.Size),
			}
			if o.Updated != "" {
				if updated, err := time.Parse(time.RFC3339, o.Updated); err == nil {
					obj.LastModified = updated
				}
			}
			objects = append(objects, obj)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	return objects, nil
}

// Copy copies an object between local and gs:// paths (in either direction or
// gs↔gs).
func (p *GCPStorageProvider) Copy(ctx context.Context, src, dst string) error {
	srcRemote, srcBucket, srcKey, err := parseGCSPath(src)
	if err != nil {
		return err
	}
	dstRemote, dstBucket, dstKey, err := parseGCSPath(dst)
	if err != nil {
		return err
	}

	switch {
	case srcRemote && dstRemote:
		return p.rewriteObject(ctx, srcBucket, srcKey, dstBucket, resolveCopyDestKey(dstKey, srcKey))

	case srcRemote && !dstRemote:
		return p.downloadObject(ctx, srcBucket, srcKey, dst)

	case !srcRemote && dstRemote:
		return p.uploadObject(ctx, src, dstBucket, resolveCopyDestKey(dstKey, src))

	default:
		return fmt.Errorf("at least one of src or dst must be a gs:// path")
	}
}

// rewriteObject copies gs:// objects with the rewrite API, looping until the
// service reports the copy complete (large objects take several calls).
func (p *GCPStorageProvider) rewriteObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	svc, err := p.newStorageService(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	var token string
	for {
		call := svc.Objects.Rewrite(srcBucket, srcKey, dstBucket, dstKey, &storage.Object{}).Context(ctx)
		if token != "" {
			call = call.RewriteToken(token)
		}
		res, err := call.Do()
		if err != nil {
			return fmt.Errorf("gs copy failed: %w", err)
		}
		if res.Done {
			return nil
		}
		if res.RewriteToken == "" {
			return fmt.Errorf("gs copy %s/%s -> %s/%s stalled: no rewrite token", srcBucket, srcKey, dstBucket, dstKey)
		}
		token = res.RewriteToken
	}
}

// uploadObject media-uploads a local file to gs://bucket/key.
func (p *GCPStorageProvider) uploadObject(ctx context.Context, localPath, bucket, key string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	svc, err := p.newStorageService(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	if _, err := svc.Objects.Insert(bucket, &storage.Object{Name: key}).Context(ctx).Media(f).Do(); err != nil {
		return fmt.Errorf("gs put failed: %w", err)
	}
	return nil
}

// downloadObject streams an object into a temp file in the destination dir and
// renames it into place only after the write is fully flushed.
func (p *GCPStorageProvider) downloadObject(ctx context.Context, bucket, key, dst string) error {
	svc, err := p.newStorageService(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	res, err := svc.Objects.Get(bucket, key).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("gs get failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	localPath := dst
	if isDir(localPath) {
		localPath = filepath.Join(localPath, filepath.Base(key))
	}
	// Stage in a temp file so a failed download can't leave a truncated file
	// behind; rename only after the body is fully written and closed.
	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".cml-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := io.Copy(tmp, res.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write local file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close local file: %w", err)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		return fmt.Errorf("finalize local file: %w", err)
	}
	return nil
}

// Sync delegates to `gcloud storage rsync` for parity and reliability.
func (p *GCPStorageProvider) Sync(ctx context.Context, src, dst string, opts *provider.SyncOptions) error {
	args := []string{"storage", "rsync", "-r", src, dst}
	if opts != nil {
		if opts.Delete {
			args = append(args, "--delete-unmatched-destination-objects")
		}
		if opts.DryRun {
			args = append(args, "--simulate")
		}
	}
	if p.client.Project() != "" {
		args = append(args, "--project", p.client.Project())
	}

	syncCmd := exec.CommandContext(ctx, "gcloud", args...)
	syncCmd.Stdin = os.Stdin
	syncCmd.Stdout = os.Stdout
	syncCmd.Stderr = os.Stderr
	return syncCmd.Run()
}

// Presign returns a V4 signed GET URL for a gs://bucket/key path. Signing is
// local, which only service-account key credentials can do.
func (p *GCPStorageProvider) Presign(ctx context.Context, path string, expirySeconds int) (string, error) {
	remote, bucket, key, err := parseGCSPath(path)
	if err != nil {
		return "", err
	}
	if !remote {
		return "", fmt.Errorf("presign requires a gs:// path")
	}

	expirySeconds, err = normalizePresignExpiry(expirySeconds)
	if err != nil {
		return "", err
	}

	creds := p.client.Credentials()
	acct, ok := parseServiceAccountKey(creds.JSON)
	if !ok {
		return "", errPresignNeedsSA
	}
	return signedV4GetURL(acct, bucket, key, expirySeconds, time.Now())
}

// maxPresignExpirySeconds is GCS's hard cap on presigned URL lifetime (7 days).
const maxPresignExpirySeconds = 604800

// normalizePresignExpiry applies the default expiry and rejects values outside
// GCS's allowed presign window (1 s .. 7 days).
func normalizePresignExpiry(seconds int) (int, error) {
	if seconds <= 0 {
		seconds = 3600
	}
	if seconds > maxPresignExpirySeconds {
		return 0, fmt.Errorf("presign expiry %ds exceeds the GCS limit of %ds (7 days)", seconds, maxPresignExpirySeconds)
	}
	return seconds, nil
}

// resolveCopyDestKey appends the source's base name when the destination key
// is empty or a directory-style prefix ending in '/'.
func resolveCopyDestKey(dstKey, srcKey string) string {
	if dstKey == "" || strings.HasSuffix(dstKey, "/") {
		dstKey = strings.TrimSuffix(dstKey, "/") + "/" + filepath.Base(srcKey)
		dstKey = strings.TrimPrefix(dstKey, "/")
	}
	return dstKey
}

// parseGCSPath returns (isRemote, bucket, key, err) for paths like
// "gs://bucket/key/path" or local file paths. An s3:// path is a scheme
// mismatch on a GCP context and errors naming the expected scheme.
func parseGCSPath(path string) (bool, string, string, error) {
	switch {
	case strings.HasPrefix(path, "gs://"):
		rest := strings.TrimPrefix(path, "gs://")
		if rest == "" {
			return true, "", "", fmt.Errorf("invalid gs path: %s", path)
		}
		parts := strings.SplitN(rest, "/", 2)
		bucket := parts[0]
		key := ""
		if len(parts) == 2 {
			key = parts[1]
		}
		if bucket == "" {
			return true, "", "", fmt.Errorf("invalid gs path: %s", path)
		}
		return true, bucket, key, nil
	case strings.HasPrefix(path, "s3://"):
		return false, "", "", fmt.Errorf("s3:// paths belong to AWS contexts; use gs:// paths on GCP")
	}
	return false, "", "", nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
