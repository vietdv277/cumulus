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
	out, err := p.client.S3().ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	buckets := make([]types.Bucket, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		bucket := types.Bucket{
			Name:     deref(b.Name),
			Provider: "aws",
		}
		if b.CreationDate != nil {
			bucket.CreatedAt = *b.CreationDate
		}
		buckets = append(buckets, bucket)
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
		_, err := p.client.S3().CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(dstBucket),
			Key:        aws.String(dstKey),
			CopySource: aws.String(srcBucket + "/" + srcKey),
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
		f, err := os.Create(localPath)
		if err != nil {
			return fmt.Errorf("create local file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(f, out.Body); err != nil {
			return fmt.Errorf("write local file: %w", err)
		}
		return nil

	case !srcRemote && dstRemote:
		f, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("open local file: %w", err)
		}
		defer func() { _ = f.Close() }()
		if dstKey == "" || strings.HasSuffix(dstKey, "/") {
			dstKey = strings.TrimSuffix(dstKey, "/") + "/" + filepath.Base(src)
			dstKey = strings.TrimPrefix(dstKey, "/")
		}
		_, err = p.client.S3().PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(dstBucket),
			Key:    aws.String(dstKey),
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

	if expirySeconds <= 0 {
		expirySeconds = 3600
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
