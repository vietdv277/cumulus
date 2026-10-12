package cmd

import (
	"testing"

	"github.com/vietdv277/cumulus/pkg/provider"
)

func TestSecretsListSource(t *testing.T) {
	tests := []struct {
		name    string
		ssmOnly bool
		smOnly  bool
		want    provider.SecretSource
		wantErr bool
	}{
		{"no flags", false, false, provider.SourceAll, false},
		{"ssm only", true, false, provider.SourceSSM, false},
		{"sm only", false, true, provider.SourceSM, false},
		{"both flags error", true, true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := secretsListSource(tt.ssmOnly, tt.smOnly)
			if (err != nil) != tt.wantErr {
				t.Fatalf("secretsListSource(%v, %v) error = %v, wantErr %v", tt.ssmOnly, tt.smOnly, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Fatalf("secretsListSource(%v, %v) = %v, want %v", tt.ssmOnly, tt.smOnly, got, tt.want)
			}
		})
	}
}
