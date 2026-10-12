package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/gcp"
)

const doctorTestConfigPath = "/xdg/cml/config.yaml"
const doctorTestHome = "/home/t"

const doctorGoodYAML = `current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
`

// doctorFileInfo is a minimal stat stub.
type doctorFileInfo struct{ mode fs.FileMode }

func (i doctorFileInfo) Name() string       { return "stub" }
func (i doctorFileInfo) Size() int64        { return 0 }
func (i doctorFileInfo) Mode() fs.FileMode  { return i.mode }
func (i doctorFileInfo) ModTime() time.Time { return time.Time{} }
func (i doctorFileInfo) IsDir() bool        { return i.mode&fs.ModeDir != 0 }
func (i doctorFileInfo) Sys() any           { return nil }

// stubEnv backs a doctorDeps with in-memory files, directories, PATH tools
// and environment variables, so doctor tests never touch real tools or files.
type stubEnv struct {
	files map[string]string      // readFile result
	dirs  map[string]fs.FileMode // stat result for directories
	modes map[string]fs.FileMode // stat result for files (default 0600)
	tools map[string]bool        // lookPath result
	env   map[string]string      // getenv result
	home  string                 // homeDir result
	aws   func(context.Context, string, string) (*aws.CallerIdentity, error)
	gcp   func(context.Context, string, string) (*gcp.CallerIdentity, error)
}

func (e *stubEnv) deps() doctorDeps {
	return doctorDeps{
		lookPath: func(name string) (string, error) {
			if e.tools[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New(name + " not found")
		},
		stat: func(path string) (fs.FileInfo, error) {
			if _, ok := e.files[path]; ok {
				mode := e.modes[path]
				if mode == 0 {
					mode = 0600
				}
				return doctorFileInfo{mode}, nil
			}
			if mode, ok := e.dirs[path]; ok {
				return doctorFileInfo{mode}, nil
			}
			return nil, fs.ErrNotExist
		},
		getenv: func(key string) string { return e.env[key] },
		readFile: func(path string) ([]byte, error) {
			if data, ok := e.files[path]; ok {
				return []byte(data), nil
			}
			return nil, fs.ErrNotExist
		},
		homeDir:     func() (string, error) { return e.home, nil },
		awsIdentity: e.aws,
		gcpIdentity: e.gcp,
	}
}

// doctorHealthy returns an environment where every local check passes
// (kubectl is installed too, so even the warn-only check is ok).
func doctorHealthy() *stubEnv {
	return &stubEnv{
		files: map[string]string{
			doctorTestConfigPath:                 doctorGoodYAML,
			doctorTestHome + "/.aws/credentials": "[prod-sso]\naws_access_key_id = A\naws_secret_access_key = s\n",
		},
		dirs:  map[string]fs.FileMode{"/xdg/cml": fs.ModeDir | 0700},
		modes: map[string]fs.FileMode{},
		tools: map[string]bool{"aws": true, "session-manager-plugin": true, "kubectl": true},
		home:  doctorTestHome,
	}
}

// doctorGCP returns a healthy GCP-only environment with ADC from the env var.
func doctorGCP() *stubEnv {
	return &stubEnv{
		files: map[string]string{
			doctorTestConfigPath: `current_context: gcp:staging
contexts:
  gcp:staging:
    provider: gcp
    project: my-project
    region: asia-southeast1
`,
			"/keys/sa.json": `{"type": "service_account"}`,
		},
		dirs:  map[string]fs.FileMode{"/xdg/cml": fs.ModeDir | 0700},
		tools: map[string]bool{"gcloud": true, "kubectl": true},
		env:   map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "/keys/sa.json"},
		home:  doctorTestHome,
	}
}

func TestDoctorRunChecks(t *testing.T) {
	dirMode := fs.ModeDir | 0700

	tests := []struct {
		name       string
		env        func() *stubEnv
		wantStatus map[string]string // check name → expected status
		wantDetail map[string]string // check name → detail substring
		wantAbsent []string
	}{
		{
			name: "healthy aws setup",
			env:  doctorHealthy,
			wantStatus: map[string]string{
				"config": doctorOK, "permissions": doctorOK, "current-context": doctorOK,
				"aliases": doctorOK, "tunnels": doctorOK,
				"aws": doctorOK, "session-manager-plugin": doctorOK,
				"profile:aws:prod": doctorOK, "kubectl": doctorOK,
			},
			wantAbsent: []string{"adc", "gcloud", "online"},
		},
		{
			name: "missing config file",
			env: func() *stubEnv {
				e := &stubEnv{dirs: map[string]fs.FileMode{"/xdg/cml": dirMode}, home: doctorTestHome}
				return e
			},
			wantStatus: map[string]string{"config": doctorFail},
			wantAbsent: []string{"permissions", "current-context"},
		},
		{
			name: "unparseable config file",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.files[doctorTestConfigPath] = "contexts: [oops\n"
				return e
			},
			wantStatus: map[string]string{"config": doctorFail, "permissions": doctorOK},
			wantAbsent: []string{"current-context"},
		},
		{
			name: "loose permissions",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.modes[doctorTestConfigPath] = 0644
				e.dirs["/xdg/cml"] = fs.ModeDir | 0755
				return e
			},
			wantStatus: map[string]string{"permissions": doctorFail},
			wantDetail: map[string]string{"permissions": "0644"},
		},
		{
			name: "no current context",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.files[doctorTestConfigPath] = `contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
`
				return e
			},
			wantStatus: map[string]string{"current-context": doctorFail},
		},
		{
			name: "dangling alias",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.files[doctorTestConfigPath] = doctorGoodYAML + "aliases:\n  old: aws:prd\n"
				return e
			},
			wantStatus: map[string]string{"aliases": doctorFail},
			wantDetail: map[string]string{"aliases": "old → aws:prd (missing)"},
		},
		{
			name: "dangling tunnel",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.files[doctorTestConfigPath] = doctorGoodYAML +
					"tunnels:\n  db-prod:\n    context: aws:prd\n    remote_port: 5432\n    local_port: 5432\n"
				return e
			},
			wantStatus: map[string]string{"tunnels": doctorFail},
			wantDetail: map[string]string{"tunnels": "db-prod → aws:prd"},
		},
		{
			name: "missing provider tools",
			env: func() *stubEnv {
				e := doctorHealthy()
				e.tools = map[string]bool{}
				return e
			},
			wantStatus: map[string]string{
				"aws": doctorFail, "session-manager-plugin": doctorFail, "kubectl": doctorWarn,
			},
		},
		{
			name: "missing aws profile",
			env: func() *stubEnv {
				e := doctorHealthy()
				delete(e.files, doctorTestHome+"/.aws/credentials")
				return e
			},
			wantStatus: map[string]string{"profile:aws:prod": doctorFail},
			wantDetail: map[string]string{"profile:aws:prod": "no readable AWS shared files"},
		},
		{
			name: "aws profile found via AWS_SHARED_CREDENTIALS_FILE",
			env: func() *stubEnv {
				e := doctorHealthy()
				delete(e.files, doctorTestHome+"/.aws/credentials")
				e.env = map[string]string{"AWS_SHARED_CREDENTIALS_FILE": "/shared/creds"}
				e.files["/shared/creds"] = "[prod-sso]\naws_access_key_id = A\n"
				return e
			},
			wantStatus: map[string]string{"profile:aws:prod": doctorOK},
			wantDetail: map[string]string{"profile:aws:prod": "/shared/creds"},
		},
		{
			name: "aws profile found in ~/.aws/config",
			env: func() *stubEnv {
				e := doctorHealthy()
				delete(e.files, doctorTestHome+"/.aws/credentials")
				e.files[doctorTestHome+"/.aws/config"] = "[profile prod-sso]\nregion = us-east-1\n"
				return e
			},
			wantStatus: map[string]string{"profile:aws:prod": doctorOK},
			wantDetail: map[string]string{"profile:aws:prod": "/.aws/config"},
		},
		{
			name: "healthy gcp setup",
			env:  doctorGCP,
			wantStatus: map[string]string{
				"config": doctorOK, "gcloud": doctorOK, "adc": doctorOK, "kubectl": doctorOK,
			},
			wantAbsent: []string{"aws", "session-manager-plugin", "profile:"},
		},
		{
			name: "gcp without adc and kubectl",
			env: func() *stubEnv {
				e := doctorGCP()
				e.tools = map[string]bool{"gcloud": true}
				delete(e.env, "GOOGLE_APPLICATION_CREDENTIALS")
				return e
			},
			wantStatus: map[string]string{"adc": doctorFail, "kubectl": doctorWarn},
			wantDetail: map[string]string{"adc": "no Application Default Credentials"},
		},
		{
			name:       "online probes absent without --online",
			env:        doctorHealthy,
			wantAbsent: []string{"online"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := runChecks(tt.env().deps(), context.Background(), doctorTestConfigPath)

			got := map[string]doctorCheck{}
			for _, c := range checks {
				got[c.Check] = c
			}
			for name, want := range tt.wantStatus {
				c, ok := got[name]
				if !ok {
					t.Fatalf("check %q did not run; ran %v", name, checkNames(checks))
				}
				if c.Status != want {
					t.Fatalf("check %q status = %q (detail %q), want %q", name, c.Status, c.Detail, want)
				}
			}
			for name, want := range tt.wantDetail {
				if !strings.Contains(got[name].Detail, want) {
					t.Fatalf("check %q detail = %q, want it to contain %q", name, got[name].Detail, want)
				}
			}
			for _, name := range tt.wantAbsent {
				if _, ok := got[name]; ok {
					t.Fatalf("check %q ran, want it absent", name)
				}
			}
		})
	}
}

func checkNames(checks []doctorCheck) []string {
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Check)
	}
	return names
}

func TestDoctorOnlineChecks(t *testing.T) {
	t.Run("aws online ok", func(t *testing.T) {
		e := doctorHealthy()
		e.aws = func(context.Context, string, string) (*aws.CallerIdentity, error) {
			return &aws.CallerIdentity{Arn: "arn:aws:iam::123:user/me"}, nil
		}
		checks := runChecks(e.deps(), context.Background(), doctorTestConfigPath)
		assertOnlineStatus(t, checks, doctorOK, "arn:aws:iam::123:user/me")
	})

	t.Run("aws online failure", func(t *testing.T) {
		e := doctorHealthy()
		e.aws = func(context.Context, string, string) (*aws.CallerIdentity, error) {
			return nil, errors.New("expired token")
		}
		checks := runChecks(e.deps(), context.Background(), doctorTestConfigPath)
		assertOnlineStatus(t, checks, doctorFail, "expired token")
	})

	t.Run("gcp online ok", func(t *testing.T) {
		e := doctorGCP()
		e.gcp = func(context.Context, string, string) (*gcp.CallerIdentity, error) {
			return &gcp.CallerIdentity{Email: "me@company.com"}, nil
		}
		checks := runChecks(e.deps(), context.Background(), doctorTestConfigPath)
		assertOnlineStatus(t, checks, doctorOK, "me@company.com")
	})

	t.Run("no probes without --online", func(t *testing.T) {
		e := doctorHealthy()
		for _, c := range runChecks(e.deps(), context.Background(), doctorTestConfigPath) {
			if c.Check == "online" {
				t.Fatal("online check ran without --online")
			}
		}
	})
}

func assertOnlineStatus(t *testing.T, checks []doctorCheck, status, detail string) {
	t.Helper()
	for _, c := range checks {
		if c.Check != "online" {
			continue
		}
		if c.Status != status || !strings.Contains(c.Detail, detail) {
			t.Fatalf("online check = %+v, want status %q with detail containing %q", c, status, detail)
		}
		return
	}
	t.Fatal("online check did not run")
}

func TestDoctorFailedCountsOnlyFailures(t *testing.T) {
	checks := []doctorCheck{
		{Status: doctorOK}, {Status: doctorWarn}, {Status: doctorFail}, {Status: doctorFail},
	}
	if got := doctorFailed(checks); got != 2 {
		t.Fatalf("doctorFailed() = %d, want 2 (warnings must not count)", got)
	}
}

func TestDoctorAWSProfileSetParsing(t *testing.T) {
	e := &stubEnv{
		files: map[string]string{
			"/creds":  "[prod-sso]\n; comment\n[dev2]\n",
			"/config": "# header\n[profile prod-sso]\nregion = us-east-1\n[default]\n",
		},
	}
	found, anyFile := awsProfileSet(e.deps(), []string{"/creds", "/config", "/missing"})
	if !anyFile {
		t.Fatal("anyFile = false, want true")
	}
	for _, name := range []string{"prod-sso", "dev2", "default"} {
		if _, ok := found[name]; !ok {
			t.Fatalf("profile %q missing from %v", name, found)
		}
	}
}

// TestDoctorJSONStdoutPurity runs `doctor -o json` through rootCmd against a
// temp config; stdout must be exactly the JSON document. The config uses an
// unknown provider so no tool/PATH checks depend on the test machine.
func TestDoctorJSONStdoutPurity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(config.GetCMLConfigDir(), 0700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(config.GetCMLConfigPath(), []byte(`current_context: aws:prod
contexts:
  aws:prod:
    provider: unused
`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	setOutputFlagState(t, "") // args provide -o json

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	rootCmd.SetArgs([]string{"doctor", "-o", "json"})
	t.Cleanup(func() { rootCmd.SetArgs([]string{}) })
	execErr := rootCmd.Execute()

	w.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}

	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	var doc []map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("stdout is not a pure JSON document: %v\ngot: %q", err, data)
	}
	if len(doc) != 5 {
		t.Fatalf("got %d checks, want 5: %v", len(doc), doc)
	}
	for i, check := range doc {
		for _, key := range []string{"check", "status", "detail", "hint"} {
			if _, ok := check[key]; !ok {
				t.Fatalf("check %d missing key %q: %v", i, key, check)
			}
		}
		if check["status"] != doctorOK {
			t.Fatalf("check %v status = %v, want ok", check["check"], check["status"])
		}
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("json output missing trailing newline: %q", data)
	}
}
