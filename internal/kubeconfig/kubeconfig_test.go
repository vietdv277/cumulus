package kubeconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kubeconfigYAML is a minimal kubeconfig; kubectl requires kind/apiVersion
// fields but this reader only needs the contexts list.
func writeKubeconfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadContextsMergesAllKubeconfigFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	missing := filepath.Join(dir, "missing.yaml")

	writeKubeconfig(t, first, `
current-context: prod
contexts:
  - name: prod
    context:
      cluster: cluster-prod
      user: user-prod
      namespace: api
  - name: dup
    context:
      cluster: cluster-from-first
      user: user-from-first
`)
	writeKubeconfig(t, second, `
current-context: dup
contexts:
  - name: dup
    context:
      cluster: cluster-from-second
      user: user-from-second
  - name: staging
    context:
      cluster: cluster-staging
      user: user-staging
`)

	// A missing file in the list must be skipped, like kubectl does.
	t.Setenv("KUBECONFIG", strings.Join([]string{first, missing, second}, string(os.PathListSeparator)))

	contexts, current, err := LoadContexts()
	if err != nil {
		t.Fatalf("LoadContexts() error = %v", err)
	}

	if current != "prod" {
		t.Errorf("current-context = %q, want prod (first file wins)", current)
	}

	got := make(map[string]Context, len(contexts))
	for _, c := range contexts {
		if _, dup := got[c.Name]; dup {
			t.Errorf("context %q returned twice", c.Name)
		}
		got[c.Name] = c
	}
	if len(got) != 3 {
		t.Fatalf("got %d unique contexts (%v), want 3", len(got), contexts)
	}
	if c := got["prod"]; c.Cluster != "cluster-prod" || c.Namespace != "api" {
		t.Errorf("prod context = %+v", c)
	}
	if c := got["dup"]; c.Cluster != "cluster-from-first" || c.User != "user-from-first" {
		t.Errorf("dup context = %+v, want fields from the first file", c)
	}
	if c := got["staging"]; c.Cluster != "cluster-staging" {
		t.Errorf("staging context = %+v", c)
	}
}

func TestLoadContextsNoKubeconfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", dir)

	contexts, current, err := LoadContexts()
	if err != nil {
		t.Fatalf("LoadContexts() error = %v", err)
	}
	if len(contexts) != 0 || current != "" {
		t.Errorf("LoadContexts() = (%v, %q), want empty", contexts, current)
	}
}
