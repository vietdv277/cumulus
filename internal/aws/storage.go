package aws

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

// AWSStorageProvider implements the StorageProvider interface for S3
type AWSStorageProvider struct {
	client  *Client
	profile string
	region  string
}

// NewStorageProvider creates a new AWS Storage provider
func NewStorageProvider(client *Client, profile, region string) *AWSStorageProvider {
	return &AWSStorageProvider{
		client:  client,
		profile: profile,
		region:  region,
	}
}

// ListBuckets returns all buckets owned by the caller
func (p *AWSStorageProvider) ListBuckets(ctx context.Context) ([]types.Bucket, error) {
	buckets := make([]types.Bucket, 0)
	paginator := s3.NewListBucketsPaginator(p.client.S3(), &s3.ListBucketsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list buckets: %w", err)
		}
		for _, b := range page.Buckets {
			bucket := types.Bucket{
				Name:     deref(b.Name),
				Provider: "aws",
			}
			if b.CreationDate != nil {
				bucket.CreatedAt = *b.CreationDate
			}
			buckets = append(buckets, bucket)
		}
	}
	return buckets, nil
}

// ListObjects returns objects under prefix in bucket
func (p *AWSStorageProvider) ListObjects(ctx context.Context, bucket, prefix string) ([]types.Object, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var objects []types.Object
	paginator := s3.NewListObjectsV2Paginator(p.client.S3(), input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		for _, o := range page.Contents {
			obj := types.Object{
				Key:          deref(o.Key),
				ETag:         strings.Trim(deref(o.ETag), "\""),
				StorageClass: string(o.StorageClass),
			}
			if o.Size != nil {
				obj.Size = *o.Size
			}
			if o.LastModified != nil {
				obj.LastModified = *o.LastModified
			}
			objects = append(objects, obj)
		}
	}
	return objects, nil
}

// Copy copies an object between local and s3 (in either direction or s3↔s3).
// Paths starting with "s3://" are treated as remote.
func (p *AWSStorageProvider) Copy(ctx context.Context, src, dst string) error {
	srcRemote, srcBucket, srcKey, err := parseS3Path(src)
	if err != nil {
		return err
	}
	dstRemote, dstBucket, dstKey, err := parseS3Path(dst)
	if err != nil {
		return err
	}

	switch {
	case srcRemote && dstRemote:
		// A single-request COPY caps at 5 GiB; look the source size up so the
		// user gets a clear error instead of an opaque API failure.
		head, err := p.client.S3().HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(srcBucket),
			Key:    aws.String(srcKey),
		})
		if err != nil {
			return fmt.Errorf("s3 source lookup failed: %w", err)
		}
		if err := checkCopySize(deref64(head.ContentLength)); err != nil {
			return err
		}

		_, err = p.client.S3().CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(dstBucket),
			Key:        aws.String(resolveCopyDestKey(dstKey, srcKey)),
			CopySource: aws.String(encodeS3CopySource(srcBucket, srcKey)),
		})
		if err != nil {
			return fmt.Errorf("s3 copy failed: %w", err)
		}
		return nil

	case srcRemote && !dstRemote:
		out, err := p.client.S3().GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(srcBucket),
			Key:    aws.String(srcKey),
		})
		if err != nil {
			return fmt.Errorf("s3 get failed: %w", err)
		}
		defer func() { _ = out.Body.Close() }()

		localPath := dst
		if isDir(localPath) {
			localPath = filepath.Join(localPath, filepath.Base(srcKey))
		}
		// Stage in a temp file so a failed download can't leave a truncated
		// file behind; rename only after the body is fully written and closed.
		tmp, err := os.CreateTemp(filepath.Dir(localPath), ".cml-download-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmp.Name()
		defer func() { _ = os.Remove(tmpPath) }()
		if _, err := io.Copy(tmp, out.Body); err != nil {
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

	case !srcRemote && dstRemote:
		f, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("open local file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if info, statErr := f.Stat(); statErr == nil {
			if err := checkCopySize(info.Size()); err != nil {
				return err
			}
		}
		_, err = p.client.S3().PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(dstBucket),
			Key:    aws.String(resolveCopyDestKey(dstKey, src)),
			Body:   f,
		})
		if err != nil {
			return fmt.Errorf("s3 put failed: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("at least one of src or dst must be an s3:// path")
	}
}

// Sync delegates to the AWS CLI's `aws s3 sync` for parity and reliability.
func (p *AWSStorageProvider) Sync(ctx context.Context, src, dst string, opts *provider.SyncOptions) error {
	args := []string{"s3", "sync", src, dst}
	if opts != nil {
		if opts.Delete {
			args = append(args, "--delete")
		}
		if opts.DryRun {
			args = append(args, "--dryrun")
		}
	}
	if p.profile != "" {
		args = append(args, "--profile", p.profile)
	}
	if p.region != "" {
		args = append(args, "--region", p.region)
	}

	syncCmd := exec.CommandContext(ctx, "aws", args...)
	syncCmd.Stdin = os.Stdin
	syncCmd.Stdout = os.Stdout
	syncCmd.Stderr = os.Stderr
	return syncCmd.Run()
}

// Presign generates a GET presigned URL for an s3://bucket/key path
func (p *AWSStorageProvider) Presign(ctx context.Context, path string, expirySeconds int) (string, error) {
	remote, bucket, key, err := parseS3Path(path)
	if err != nil {
		return "", err
	}
	if !remote {
		return "", fmt.Errorf("presign requires an s3:// path")
	}

	expirySeconds, err = normalizePresignExpiry(expirySeconds)
	if err != nil {
		return "", err
	}

	presigner := s3.NewPresignClient(p.client.S3())
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(time.Duration(expirySeconds)*time.Second))
	if err != nil {
		return "", fmt.Errorf("presign failed: %w", err)
	}
	return req.URL, nil
}

// maxPresignExpirySeconds is S3's hard cap on presigned URL lifetime (7 days).
const maxPresignExpirySeconds = 604800

// normalizePresignExpiry applies the default expiry and rejects values outside
// S3's allowed presign window.
func normalizePresignExpiry(seconds int) (int, error) {
	if seconds <= 0 {
		seconds = 3600
	}
	if seconds > maxPresignExpirySeconds {
		return 0, fmt.Errorf("presign expiry %ds exceeds the S3 limit of %ds (7 days)", seconds, maxPresignExpirySeconds)
	}
	return seconds, nil
}

// maxSingleObjectCopy is S3's single-request PUT/COPY limit (5 GiB).
const maxSingleObjectCopy = 5 * 1024 * 1024 * 1024

// checkCopySize returns a clear error for sources above the single-request
// limit; multipart upload/copy is not supported yet.
func checkCopySize(size int64) error {
	if size > maxSingleObjectCopy {
		return fmt.Errorf("%d-byte object exceeds the 5 GiB single-request copy limit; multipart upload is not supported yet", size)
	}
	return nil
}

// encodeS3CopySource returns the x-amz-copy-source value for bucket/key, with
// the key URL-encoded per S3 rules — every byte outside the unreserved ASCII
// set, except the '/' separators.
func encodeS3CopySource(bucket, key string) string {
	var b strings.Builder
	b.WriteString(bucket)
	b.WriteString("/")
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c == '/' || c == '-' || c == '_' || c == '.' || c == '~',
			c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
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

// deref64 safely dereferences an int64 pointer
func deref64(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// parseS3Path returns (isRemote, bucket, key, err) for paths like
// "s3://bucket/key/path" or local file paths.
func parseS3Path(path string) (bool, string, string, error) {
	if !strings.HasPrefix(path, "s3://") {
		return false, "", "", nil
	}
	rest := strings.TrimPrefix(path, "s3://")
	if rest == "" {
		return true, "", "", fmt.Errorf("invalid s3 path: %s", path)
	}
	parts := strings.SplitN(rest, "/", 2)
	bucket := parts[0]
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	if bucket == "" {
		return true, "", "", fmt.Errorf("invalid s3 path: %s", path)
	}
	return true, bucket, key, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
