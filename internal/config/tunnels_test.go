package config

import (
	"strings"
	"testing"
)

func TestTunnelSaveListDeleteRoundTrip(t *testing.T) {
	isolateHome(t)

	tunnels := map[string]*TunnelConfig{
		"web-ssh": {Context: "aws:prod", Target: "i-0web123", RemotePort: 22, LocalPort: 2222},
		"db-prod": {Context: "aws:prod", Target: "i-0db123", RemotePort: 5432, LocalPort: 5432},
	}
	// Save in non-alphabetical order to prove ListTunnels sorts.
	for _, name := range []string{"web-ssh", "db-prod"} {
		if err := SaveTunnel(name, tunnels[name], false); err != nil {
			t.Fatalf("SaveTunnel(%q) error = %v", name, err)
		}
	}

	names, got, err := ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels() error = %v", err)
	}
	if len(names) != 2 || names[0] != "db-prod" || names[1] != "web-ssh" {
		t.Fatalf("names = %v, want [db-prod web-ssh]", names)
	}
	if got["db-prod"].Target != "i-0db123" || got["db-prod"].RemotePort != 5432 || got["db-prod"].LocalPort != 5432 {
		t.Errorf("db-prod = %+v, want the saved fields", got["db-prod"])
	}

	if err := DeleteTunnel("db-prod"); err != nil {
		t.Fatalf("DeleteTunnel() error = %v", err)
	}
	names, got, err = ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels() error = %v", err)
	}
	if len(names) != 1 || names[0] != "web-ssh" {
		t.Fatalf("names after delete = %v, want [web-ssh]", names)
	}
	if _, ok := got["db-prod"]; ok {
		t.Error("deleted tunnel still present in config")
	}

	if err := DeleteTunnel("db-prod"); err == nil {
		t.Error("expected error deleting an unknown tunnel")
	}
}

func TestDeleteTunnelSuggestions(t *testing.T) {
	t.Run("typo suggests tunnel names", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := DeleteTunnel("db-prd")
		if err == nil {
			t.Fatal("expected error deleting an unknown tunnel")
		}
		want := `tunnel "db-prd" not found — did you mean "db-prod"?`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("no close match keeps today's text", func(t *testing.T) {
		isolateHome(t)
		writeTestFile(t, GetCMLConfigPath(), suggestTestYAML, 0600)

		err := DeleteTunnel("zzzzzz")
		if err == nil {
			t.Fatal("expected error deleting an unknown tunnel")
		}
		want := `tunnel "zzzzzz" not found`
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})
}

func TestSaveTunnelOverwriteGuard(t *testing.T) {
	isolateHome(t)

	first := &TunnelConfig{Context: "aws:prod", Target: "i-0old123", RemotePort: 5432, LocalPort: 5432}
	second := &TunnelConfig{Context: "gcp:staging", Target: "my-db", RemotePort: 5432, LocalPort: 15432}

	if err := SaveTunnel("db-prod", first, false); err != nil {
		t.Fatalf("first SaveTunnel() error = %v", err)
	}
	if err := SaveTunnel("db-prod", second, false); err == nil {
		t.Fatal("expected error saving over an existing tunnel without overwrite")
	}
	// The failed save must not have touched the stored entry.
	tunnel, err := GetTunnelConfig("db-prod")
	if err != nil {
		t.Fatalf("GetTunnelConfig() error = %v", err)
	}
	if tunnel.Target != "i-0old123" {
		t.Errorf("target = %q, want the original i-0old123", tunnel.Target)
	}

	if err := SaveTunnel("db-prod", second, true); err != nil {
		t.Fatalf("SaveTunnel with overwrite error = %v", err)
	}
	tunnel, err = GetTunnelConfig("db-prod")
	if err != nil {
		t.Fatalf("GetTunnelConfig() error = %v", err)
	}
	if tunnel.Target != "my-db" || tunnel.LocalPort != 15432 {
		t.Errorf("tunnel = %+v, want the overwritten fields", tunnel)
	}
}

func TestSaveTunnelValidates(t *testing.T) {
	tests := []struct {
		name       string
		tunnelName string
		tunnel     *TunnelConfig
		wantValid  bool
		wantErr    string
	}{
		{name: "leading digit", tunnelName: "0first", tunnel: &TunnelConfig{Context: "aws:prod"}, wantValid: true},
		{name: "dashes", tunnelName: "db-prod-2", tunnel: &TunnelConfig{Context: "aws:prod"}, wantValid: true},
		{name: "empty", tunnelName: "", tunnel: &TunnelConfig{Context: "aws:prod"}, wantErr: "invalid tunnel name"},
		{name: "leading dash", tunnelName: "-lead", tunnel: &TunnelConfig{Context: "aws:prod"}, wantErr: "invalid tunnel name"},
		{name: "uppercase", tunnelName: "Big", tunnel: &TunnelConfig{Context: "aws:prod"}, wantErr: "invalid tunnel name"},
		{name: "underscore", tunnelName: "has_underscore", tunnel: &TunnelConfig{Context: "aws:prod"}, wantErr: "invalid tunnel name"},
		{name: "space", tunnelName: "has space", tunnel: &TunnelConfig{Context: "aws:prod"}, wantErr: "invalid tunnel name"},
		{name: "no context", tunnelName: "db-prod", tunnel: &TunnelConfig{}, wantErr: "needs a context"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateHome(t)

			err := SaveTunnel(tt.tunnelName, tt.tunnel, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := GetTunnelConfig(tt.tunnelName); err != nil {
				t.Fatalf("saved tunnel %q not found: %v", tt.tunnelName, err)
			}
		})
	}
}
