package cmd

import (
	"strings"
	"testing"
)

func TestParseLsTarget(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		scheme  string
		bucket  string
		prefix  string
		wantErr string
	}{
		{"s3 bucket only", []string{"s3://bkt"}, "s3://", "bkt", "", ""},
		{"s3 bucket with prefix in uri", []string{"s3://bkt/logs/"}, "s3://", "bkt", "logs/", ""},
		{"s3 prefix as positional", []string{"s3://bkt", "logs/"}, "s3://", "bkt", "logs/", ""},
		{"gs bucket", []string{"gs://bkt"}, "gs://", "bkt", "", ""},
		{"gs with positional prefix", []string{"gs://bkt", "logs/"}, "gs://", "bkt", "logs/", ""},
		{"scheme mismatch", []string{"s3://bkt"}, "gs://", "", "", "this context uses gs:// paths"},
		{"scheme mismatch reversed", []string{"gs://bkt"}, "s3://", "", "", "this context uses s3:// paths"},
		{"no scheme", []string{"bkt"}, "s3://", "", "", "bucket must be a s3:// URI"},
		{"empty bucket", []string{"s3:///key"}, "s3://", "", "", "invalid s3:// URI"},
		{"prefix twice", []string{"s3://bkt/logs/", "more/"}, "s3://", "", "", "prefix specified twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix, err := parseLsTarget(tt.args, tt.scheme)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseLsTarget(%v, %q) error = nil, want %q", tt.args, tt.scheme, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("parseLsTarget(%v, %q) error = %q, want %q", tt.args, tt.scheme, err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLsTarget(%v, %q) unexpected error: %v", tt.args, tt.scheme, err)
			}
			if bucket != tt.bucket || prefix != tt.prefix {
				t.Errorf("parseLsTarget(%v, %q) = (%q, %q), want (%q, %q)",
					tt.args, tt.scheme, bucket, prefix, tt.bucket, tt.prefix)
			}
		})
	}
}

func TestStorageScheme(t *testing.T) {
	if got := storageScheme("gcp"); got != "gs://" {
		t.Errorf("storageScheme(gcp) = %q, want gs://", got)
	}
	if got := storageScheme("aws"); got != "s3://" {
		t.Errorf("storageScheme(aws) = %q, want s3://", got)
	}
	if got := storageScheme("unknown"); got != "s3://" {
		t.Errorf("storageScheme(unknown) = %q, want s3://", got)
	}
}
