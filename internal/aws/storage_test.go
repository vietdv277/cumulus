package aws

import (
	"strings"
	"testing"
)

func TestEncodeS3CopySource(t *testing.T) {
	tests := []struct {
		bucket string
		key    string
		want   string
	}{
		{"bkt", "plain/key.txt", "bkt/plain/key.txt"},
		{"bkt", "my key.txt", "bkt/my%20key.txt"},
		{"bkt", "a+b.txt", "bkt/a%2Bb.txt"},
		{"bkt", "100%done", "bkt/100%25done"},
		{"bkt", "café/menu.txt", "bkt/caf%C3%A9/menu.txt"},
		{"bkt", "dir/with space/名前.txt", "bkt/dir/with%20space/%E5%90%8D%E5%89%8D.txt"},
		{"bkt", "a&b=c?d", "bkt/a%26b%3Dc%3Fd"},
		{"bkt", "", "bkt/"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := encodeS3CopySource(tt.bucket, tt.key); got != tt.want {
				t.Errorf("encodeS3CopySource(%q, %q) = %q, want %q", tt.bucket, tt.key, got, tt.want)
			}
		})
	}
}

func TestResolveCopyDestKey(t *testing.T) {
	tests := []struct {
		name   string
		dstKey string
		srcKey string
		want   string
	}{
		{"s3 dir key ending in slash", "reports/2024/", "data.csv", "reports/2024/data.csv"},
		{"empty dst uses source base", "", "data.csv", "data.csv"},
		{"bare slash", "/", "data.csv", "data.csv"},
		{"full dst key is kept", "reports/backup.csv", "data.csv", "reports/backup.csv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCopyDestKey(tt.dstKey, tt.srcKey); got != tt.want {
				t.Errorf("resolveCopyDestKey(%q, %q) = %q, want %q", tt.dstKey, tt.srcKey, got, tt.want)
			}
		})
	}
}

func TestNormalizePresignExpiry(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    int
		wantErr string
	}{
		{"zero defaults to 1h", 0, 3600, ""},
		{"negative defaults to 1h", -5, 3600, ""},
		{"valid value kept", 60, 60, ""},
		{"max 7 days accepted", 604800, 604800, ""},
		{"over 7 days rejected", 604801, 0, "exceeds the S3 limit"},
		{"far over 7 days rejected", 100000000, 0, "exceeds the S3 limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePresignExpiry(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("normalizePresignExpiry(%d) unexpected error: %v", tt.in, err)
				}
				if got != tt.want {
					t.Errorf("normalizePresignExpiry(%d) = %d, want %d", tt.in, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("normalizePresignExpiry(%d) error = nil, want %q", tt.in, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("normalizePresignExpiry(%d) error = %q, want %q", tt.in, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCheckCopySize(t *testing.T) {
	const fiveGiB = 5 * 1024 * 1024 * 1024

	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"small object ok", 1024, false},
		{"exactly 5 GiB ok", fiveGiB, false},
		{"over 5 GiB rejected", fiveGiB + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCopySize(tt.size)
			if tt.wantErr && err == nil {
				t.Errorf("checkCopySize(%d) error = nil, want error", tt.size)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("checkCopySize(%d) unexpected error: %v", tt.size, err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "multipart") {
				t.Errorf("checkCopySize(%d) error = %q, want multipart hint", tt.size, err.Error())
			}
		})
	}
}
