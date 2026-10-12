package aws

import (
	"strings"
	"testing"
)

func TestResolveSecretRoute(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantBackend  SecretBackend
		wantStripped string
		wantFallback bool
	}{
		{"leading slash goes to SSM", "/a/b", BackendSSM, "/a/b", false},
		{"explicit ssm prefix is stripped", "ssm:x", BackendSSM, "x", false},
		{"explicit sm prefix with path is stripped", "sm:/prod/x", BackendSM, "/prod/x", false},
		{"plain name defaults to SM with SSM fallback", "plain-name", BackendSM, "plain-name", true},
		{"ssm prefix wins over leading slash", "ssm:/a/b", BackendSSM, "/a/b", false},
		{"empty name defaults to SM", "", BackendSM, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, stripped, fallback := resolveSecretRoute(tt.input)
			if backend != tt.wantBackend || stripped != tt.wantStripped || fallback != tt.wantFallback {
				t.Fatalf("resolveSecretRoute(%q) = (%v, %q, %v), want (%v, %q, %v)",
					tt.input, backend, stripped, fallback, tt.wantBackend, tt.wantStripped, tt.wantFallback)
			}
		})
	}
}

func TestSSMDeleteHint(t *testing.T) {
	err := ssmDeleteHint("db-pass")
	want := `secret "db-pass" not found in Secrets Manager; an SSM parameter with that name exists — run 'cml secrets delete ssm:db-pass' to delete it permanently`
	if err.Error() != want {
		t.Fatalf("ssmDeleteHint() = %q, want %q", err.Error(), want)
	}
	// The hint must point at the explicit ssm: prefix so the SSM parameter is
	// only deleted after an explicit opt-in.
	if !strings.Contains(err.Error(), "cml secrets delete ssm:db-pass") {
		t.Fatalf("ssmDeleteHint() does not name the ssm: command: %q", err.Error())
	}
}
