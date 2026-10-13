package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/pkg/provider"
)

// fakeVMProvider is a VMProvider whose Tunnel only records its arguments;
// the embedded interface supplies the other methods (they are never called).
type fakeVMProvider struct {
	provider.VMProvider
	target string
	opts   *provider.TunnelOptions
}

func (f *fakeVMProvider) Tunnel(_ context.Context, target string, opts *provider.TunnelOptions) error {
	f.target = target
	f.opts = opts
	return nil
}

// injectTunnelFactory swaps the tunnel provider factory for a recording fake.
func injectTunnelFactory(t *testing.T, fake *fakeVMProvider) *struct {
	ctxName  string
	provider string
} {
	t.Helper()
	got := &struct {
		ctxName  string
		provider string
	}{}
	prev := tunnelProviderFactory
	tunnelProviderFactory = func(_ context.Context, ctxConfig *config.Context, ctxName string) (provider.VMProvider, error) {
		got.ctxName = ctxName
		got.provider = ctxConfig.Provider
		return fake, nil
	}
	t.Cleanup(func() { tunnelProviderFactory = prev })
	return got
}

const tunnelTestConfigYAML = `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
  gcp:staging:
    provider: gcp
    project: my-project
    region: asia-southeast1
tunnels:
  db-stg:
    context: gcp:staging
    target: my-db
    remote_port: 5432
    local_port: 5432
  via-bastion:
    context: aws:prod
    target: i-0target
    bastion: i-0bastion
    remote_host: db.internal
    remote_port: 5432
    local_port: 5432
`

func TestTunnelStartUsesTunnelsOwnContext(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	fake := &fakeVMProvider{}
	got := injectTunnelFactory(t, fake)

	if err := runTunnelStart(tunnelStartCmd, []string{"db-stg"}); err != nil {
		t.Fatalf("runTunnelStart() error = %v", err)
	}

	// The current context is aws:prod but the tunnel was saved with gcp:staging.
	if got.ctxName != "gcp:staging" || got.provider != "gcp" {
		t.Fatalf("factory got context %q provider %q, want gcp:staging/gcp", got.ctxName, got.provider)
	}
	if fake.target != "my-db" {
		t.Errorf("target = %q, want my-db", fake.target)
	}
	if fake.opts == nil || fake.opts.RemotePort != 5432 || fake.opts.LocalPort != 5432 {
		t.Errorf("opts = %+v, want the saved ports", fake.opts)
	}
}

func TestTunnelStartLocalPortOverride(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	fake := &fakeVMProvider{}
	injectTunnelFactory(t, fake)

	flag := tunnelStartCmd.Flags().Lookup("local-port")
	flag.Value.Set("25432")
	flag.Changed = true
	t.Cleanup(func() {
		flag.Value.Set("5432")
		flag.Changed = false
	})

	if err := runTunnelStart(tunnelStartCmd, []string{"db-stg"}); err != nil {
		t.Fatalf("runTunnelStart() error = %v", err)
	}
	if fake.opts == nil || fake.opts.LocalPort != 25432 || fake.opts.RemotePort != 5432 {
		t.Errorf("opts = %+v, want local 25432 -> remote 5432", fake.opts)
	}
}

func TestTunnelStartRejectsBastionAndRemoteHost(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	fake := &fakeVMProvider{}
	injectTunnelFactory(t, fake)

	err := runTunnelStart(tunnelStartCmd, []string{"via-bastion"})
	if err == nil {
		t.Fatal("expected error for a bastion/remote_host tunnel")
	}
	if !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("err = %v, want a not-supported-yet error", err)
	}
	if fake.target != "" {
		t.Error("a provider was built for an unsupported tunnel")
	}
}

func TestTunnelStartUnknownTunnel(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	err := runTunnelStart(tunnelStartCmd, []string{"nope"})
	if err == nil || !strings.Contains(err.Error(), `tunnel "nope" not found`) {
		t.Fatalf("err = %v, want a not-found error", err)
	}
}

func TestTunnelStartUnknownTunnelSuggests(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	err := runTunnelStart(tunnelStartCmd, []string{"db-stg2"})
	if err == nil {
		t.Fatal("expected error for an unknown tunnel")
	}
	want := `tunnel "db-stg2" not found — did you mean "db-stg"?`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestTunnelRmUnknownTunnelSuggests(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	err := runTunnelRm(tunnelRmCmd, []string{"db-stg2"})
	if err == nil {
		t.Fatal("expected error for an unknown tunnel")
	}
	want := `tunnel "db-stg2" not found — did you mean "db-stg"?`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}

	err = runTunnelRm(tunnelRmCmd, []string{"zzzzzz"})
	if err == nil {
		t.Fatal("expected error for an unknown tunnel")
	}
	want = `tunnel "zzzzzz" not found`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestVMSaveOverwriteGuardFailsBeforeProvider(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	prevSave, prevForce := vmTunnelSave, vmTunnelForce
	t.Cleanup(func() { vmTunnelSave, vmTunnelForce = prevSave, prevForce })

	// db-stg already exists in the config; a --save without --force must fail
	// before any provider is created (no network, no AWS client).
	vmTunnelSave = "db-stg"
	vmTunnelForce = false
	err := saveVMTunnel("i-0new123", 5432, 5432)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want an already-exists error", err)
	}

	// The stored entry is untouched.
	tunnel, err := config.GetTunnelConfig("db-stg")
	if err != nil {
		t.Fatalf("GetTunnelConfig() error = %v", err)
	}
	if tunnel.Target != "my-db" {
		t.Errorf("target = %q, want the original my-db", tunnel.Target)
	}

	// --force overwrites and records the resolved context name.
	vmTunnelForce = true
	if err := saveVMTunnel("i-0new123", 5432, 25432); err != nil {
		t.Fatalf("saveVMTunnel with --force error = %v", err)
	}
	tunnel, err = config.GetTunnelConfig("db-stg")
	if err != nil {
		t.Fatalf("GetTunnelConfig() error = %v", err)
	}
	if tunnel.Context != "aws:prod" || tunnel.Target != "i-0new123" || tunnel.LocalPort != 25432 {
		t.Errorf("tunnel = %+v, want the overwritten fields with the current context", tunnel)
	}
}

func TestVMSaveRejectsInvalidName(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	prevSave := vmTunnelSave
	t.Cleanup(func() { vmTunnelSave = prevSave })

	vmTunnelSave = "Not-Valid"
	if err := saveVMTunnel("i-0new123", 5432, 5432); err == nil {
		t.Fatal("expected error for an invalid tunnel name")
	}
}

func TestTunnelRmRemovesSavedTunnel(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	if err := runTunnelRm(tunnelRmCmd, []string{"db-stg"}); err != nil {
		t.Fatalf("runTunnelRm() error = %v", err)
	}
	if _, err := config.GetTunnelConfig("db-stg"); err == nil {
		t.Fatal("expected the removed tunnel to be gone")
	}

	// Removing it again is an error.
	if err := runTunnelRm(tunnelRmCmd, []string{"db-stg"}); err == nil {
		t.Fatal("expected error removing an unknown tunnel")
	}
}

// executeForError runs rootCmd with the given args and returns cobra's error,
// with usage/error printing silenced; resets the legacy flags afterwards.
func executeForError(t *testing.T, args []string) error {
	t.Helper()
	rootCmd.SilenceUsage = true
	t.Cleanup(func() {
		rootCmd.SilenceUsage = false
		resetLegacyFlags()
	})

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error for %v", args)
	}
	return err
}

func TestTunnelStartRejectsProfileFlag(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	// "nope" is not a saved tunnel either — the exact guard message proves the
	// rejection happened before any config lookup or provider creation.
	err := executeForError(t, []string{"tunnel", "start", "nope", "-p", "dev"})
	want := "--profile is not supported by context-aware commands; use --context <name> or 'cml use <context>'"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

func TestAWSWhoamiRejectsRegionFlag(t *testing.T) {
	setTestConfig(t, tunnelTestConfigYAML)

	// The exact guard message proves the failure happened before any AWS
	// client was built (no network call).
	err := executeForError(t, []string{"aws", "iam", "whoami", "-r", "us-west-2"})
	want := "--region is not supported by context-aware commands; use --context <name> or 'cml use <context>'"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
