package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/vietdv277/cumulus/internal/aws"
	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/errhint"
	"github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/ui"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the local setup for common problems",
	Long: `Diagnose common setup problems: the config file, the current context,
dangling aliases, saved tunnels, per-provider tools on PATH, the AWS shared
profile files and GCP Application Default Credentials.

Local checks never touch the network. Pass --online to additionally verify
the current context's credentials for real (STS GetCallerIdentity for AWS,
a GCP token fetch).

Examples:
  cml doctor
  cml doctor --online
  cml doctor -o json`,
	RunE: runDoctor,
}

var doctorOnline bool

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorOnline, "online", false, "Also verify cloud authentication for the current context (network)")
}

// Check statuses.
const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
)

// doctorCheck is one diagnostic result — the stable json/yaml shape.
type doctorCheck struct {
	Check  string `json:"check" yaml:"check"`
	Status string `json:"status" yaml:"status"`
	Detail string `json:"detail" yaml:"detail"`
	Hint   string `json:"hint" yaml:"hint"`
}

// doctorDeps injects the environment so the checks are pure functions: tests
// stub these instead of installing real tools or writing real files. The
// online probes stay nil unless --online was requested.
type doctorDeps struct {
	lookPath func(string) (string, error)
	stat     func(string) (fs.FileInfo, error)
	getenv   func(string) string
	readFile func(string) ([]byte, error)
	homeDir  func() (string, error)

	awsIdentity func(context.Context, string, string) (*aws.CallerIdentity, error)
	gcpIdentity func(context.Context, string, string) (*gcp.CallerIdentity, error)
}

// newDoctorDeps returns the real, environment-backed dependencies.
func newDoctorDeps() doctorDeps {
	return doctorDeps{
		lookPath: exec.LookPath,
		stat:     os.Stat,
		getenv:   os.Getenv,
		readFile: os.ReadFile,
		homeDir:  os.UserHomeDir,
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	deps := newDoctorDeps()
	if doctorOnline {
		deps.awsIdentity = aws.GetCallerIdentity
		deps.gcpIdentity = gcp.GetCallerIdentity
	}

	checks := runChecks(deps, cmd.Context(), config.GetCMLConfigPath())

	if err := render(os.Stdout, out, checks, func() { printDoctorTable(checks) }); err != nil {
		return err
	}
	if failed := doctorFailed(checks); failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}

// doctorFailed counts the failed checks; warnings do not fail.
func doctorFailed(checks []doctorCheck) int {
	n := 0
	for _, c := range checks {
		if c.Status == doctorFail {
			n++
		}
	}
	return n
}

// runChecks runs every doctor check against the injected environment.
// configPath is passed in so tests can point at their own files.
func runChecks(deps doctorDeps, ctx context.Context, configPath string) []doctorCheck {
	checks := make([]doctorCheck, 0, 10)
	add := func(c doctorCheck) { checks = append(checks, c) }

	info, statErr := deps.stat(configPath)
	if statErr != nil {
		add(doctorCheck{
			Check: "config", Status: doctorFail,
			Detail: fmt.Sprintf("config file not found at %s", configPath),
			Hint:   "it is created on first save, e.g. 'cml use add aws:prod --profile <p> --region <r>'",
		})
		return checks
	}

	// Everything else needs a parsed config, so a broken file ends the run
	// after the still-meaningful permission check.
	data, readErr := deps.readFile(configPath)
	var cfg *config.CMLConfig
	if readErr == nil {
		cfg = &config.CMLConfig{}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			readErr = err
			cfg = nil
		}
	}
	if readErr != nil {
		add(doctorCheck{
			Check: "config", Status: doctorFail,
			Detail: fmt.Sprintf("config file at %s could not be read or parsed: %v", configPath, readErr),
			Hint:   "fix or remove the file; a fresh config is created on the next save",
		})
		add(checkConfigPermissions(deps, configPath, info))
		return checks
	}
	add(doctorCheck{Check: "config", Status: doctorOK, Detail: fmt.Sprintf("parsed %s", configPath)})
	add(checkConfigPermissions(deps, configPath, info))

	add(checkCurrentContext(cfg))
	add(checkAliases(cfg))
	add(checkTunnels(cfg))

	// Tool checks for the providers actually configured.
	providers := map[string]bool{}
	for _, ctxCfg := range cfg.Contexts {
		if ctxCfg != nil {
			providers[ctxCfg.Provider] = true
		}
	}
	if providers["aws"] {
		add(toolCheck(deps, "aws", "aws", "install the AWS CLI — see docs.aws.amazon.com/cli", false))
		add(toolCheck(deps, "session-manager-plugin", "session-manager-plugin",
			"install the AWS Session Manager plugin (needed for SSM port-forwarding)", false))
	}
	if providers["gcp"] {
		add(toolCheck(deps, "gcloud", "gcloud", "install the gcloud CLI", false))
		add(checkADC(deps))
	}
	if providers["aws"] || providers["gcp"] {
		add(toolCheck(deps, "kubectl", "kubectl", "install kubectl for the k8s commands", true))
	}
	if providers["aws"] {
		for _, name := range sortedProviderContexts(cfg, "aws") {
			add(checkAWSProfile(deps, name, cfg.Contexts[name].Profile))
		}
	}

	checks = append(checks, checkOnline(deps, ctx, cfg)...)
	return checks
}

func checkConfigPermissions(deps doctorDeps, configPath string, info fs.FileInfo) doctorCheck {
	var problems []string
	if mode := info.Mode().Perm(); mode != 0600 {
		problems = append(problems, fmt.Sprintf("config file is %04o (want 0600)", mode))
	}
	if dirInfo, err := deps.stat(filepath.Dir(configPath)); err != nil {
		problems = append(problems, fmt.Sprintf("config directory not readable: %v", err))
	} else if mode := dirInfo.Mode().Perm(); mode != 0700 {
		problems = append(problems, fmt.Sprintf("config directory is %04o (want 0700)", mode))
	}
	if len(problems) == 0 {
		return doctorCheck{Check: "permissions", Status: doctorOK, Detail: "config file 0600, directory 0700"}
	}
	return doctorCheck{
		Check:  "permissions",
		Status: doctorFail,
		Detail: strings.Join(problems, "; "),
		Hint:   "any command that saves the config fixes this, e.g. 'cml use update <context> --region <r>'",
	}
}

func checkCurrentContext(cfg *config.CMLConfig) doctorCheck {
	c := doctorCheck{Check: "current-context", Status: doctorOK, Detail: cfg.CurrentContext}
	switch cfg.CurrentContext {
	case "":
		c.Status = doctorFail
		c.Detail = "no current context set"
		c.Hint = "cml use <context>"
	default:
		if _, ok := cfg.Contexts[cfg.CurrentContext]; !ok {
			c.Status = doctorFail
			c.Detail = fmt.Sprintf("current context %q does not exist", cfg.CurrentContext)
			c.Hint = "'cml use <context>' or 'cml use add <context> ...' to recreate it"
		}
	}
	return c
}

func checkAliases(cfg *config.CMLConfig) doctorCheck {
	dangling := config.DanglingAliases(cfg)
	if len(dangling) == 0 {
		return doctorCheck{Check: "aliases", Status: doctorOK,
			Detail: fmt.Sprintf("%d alias(es) point at existing contexts", len(cfg.Aliases))}
	}

	names := make([]string, 0, len(dangling))
	for name := range dangling {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s → %s (missing)", name, dangling[name])
	}
	return doctorCheck{
		Check:  "aliases",
		Status: doctorFail,
		Detail: fmt.Sprintf("%d dangling alias(es): %s", len(names), strings.Join(parts, ", ")),
		Hint:   "'cml alias rm <name>' or 'cml alias add <name> <context> --force' to re-point it",
	}
}

func checkTunnels(cfg *config.CMLConfig) doctorCheck {
	if len(cfg.Tunnels) == 0 {
		return doctorCheck{Check: "tunnels", Status: doctorOK, Detail: "no saved tunnels"}
	}

	names := make([]string, 0, len(cfg.Tunnels))
	for name := range cfg.Tunnels {
		names = append(names, name)
	}
	sort.Strings(names)

	var dangling []string
	for _, name := range names {
		tunnel := cfg.Tunnels[name]
		if tunnel == nil || tunnel.Context == "" {
			continue
		}
		if _, ok := cfg.Contexts[tunnel.Context]; !ok {
			dangling = append(dangling, fmt.Sprintf("%s → %s", name, tunnel.Context))
		}
	}
	if len(dangling) == 0 {
		return doctorCheck{Check: "tunnels", Status: doctorOK,
			Detail: fmt.Sprintf("%d saved tunnel(s) point at existing contexts", len(cfg.Tunnels))}
	}
	return doctorCheck{
		Check:  "tunnels",
		Status: doctorFail,
		Detail: fmt.Sprintf("%d saved tunnel(s) point at missing contexts: %s", len(dangling), strings.Join(dangling, ", ")),
		Hint:   "update or remove the tunnel in the config file",
	}
}

// toolCheck reports whether binary is on PATH; warn-only checks stay warnings.
func toolCheck(deps doctorDeps, checkName, binary, hint string, warnOnly bool) doctorCheck {
	c := doctorCheck{Check: checkName, Status: doctorOK, Detail: binary + " found on PATH"}
	if _, err := deps.lookPath(binary); err != nil {
		c.Status = doctorWarn
		if !warnOnly {
			c.Status = doctorFail
		}
		c.Detail = fmt.Sprintf("%s not found on PATH", binary)
		c.Hint = hint
	}
	return c
}

// checkADC verifies Application Default Credentials are discoverable from the
// environment or the well-known gcloud file — no network call.
func checkADC(deps doctorDeps) doctorCheck {
	if path := deps.getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		data, err := deps.readFile(path)
		if err != nil {
			return doctorCheck{
				Check: "adc", Status: doctorFail,
				Detail: fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS file %s is not readable", path),
				Hint:   "fix the path, or unset it and run 'gcloud auth application-default login'",
			}
		}
		var probe map[string]any
		if err := json.Unmarshal(data, &probe); err != nil {
			return doctorCheck{
				Check: "adc", Status: doctorFail,
				Detail: fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS file %s is not valid JSON", path),
				Hint:   "point it at a service-account key file",
			}
		}
		return doctorCheck{Check: "adc", Status: doctorOK,
			Detail: fmt.Sprintf("ADC from GOOGLE_APPLICATION_CREDENTIALS: %s", path)}
	}

	home, err := deps.homeDir()
	if err != nil {
		return doctorCheck{Check: "adc", Status: doctorFail,
			Detail: fmt.Sprintf("cannot resolve home directory: %v", err)}
	}
	path := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	if _, err := deps.stat(path); err != nil {
		return doctorCheck{
			Check: "adc", Status: doctorFail,
			Detail: "no Application Default Credentials found",
			Hint:   "gcloud auth application-default login",
		}
	}
	return doctorCheck{Check: "adc", Status: doctorOK, Detail: fmt.Sprintf("ADC found at %s", path)}
}

// awsSectionRe matches [name] (credentials file) and [profile name] / [default]
// (config file).
var awsSectionRe = regexp.MustCompile(`^\[(?:profile\s+)?([^\]]+)\]$`)

// awsSharedPaths returns the AWS shared config/credentials paths, honouring
// AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE.
func awsSharedPaths(deps doctorDeps) []string {
	home, err := deps.homeDir()
	if err != nil {
		return nil
	}
	configPath := deps.getenv("AWS_CONFIG_FILE")
	if configPath == "" {
		configPath = filepath.Join(home, ".aws", "config")
	}
	credPath := deps.getenv("AWS_SHARED_CREDENTIALS_FILE")
	if credPath == "" {
		credPath = filepath.Join(home, ".aws", "credentials")
	}
	return []string{configPath, credPath}
}

// awsProfileSet parses the given AWS INI files locally and returns profile
// name → defining file. Unreadable files are skipped.
func awsProfileSet(deps doctorDeps, paths []string) (found map[string]string, anyFile bool) {
	found = make(map[string]string)
	for _, path := range paths {
		data, err := deps.readFile(path)
		if err != nil {
			continue
		}
		anyFile = true
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			if m := awsSectionRe.FindStringSubmatch(line); m != nil {
				found[strings.TrimSpace(m[1])] = path
			}
		}
	}
	return found, anyFile
}

// checkAWSProfile verifies the profile of one AWS context against the shared
// files, parsed locally — no network.
func checkAWSProfile(deps doctorDeps, ctxName, profile string) doctorCheck {
	checkName := "profile:" + ctxName
	if profile == "" {
		return doctorCheck{
			Check: checkName, Status: doctorFail,
			Detail: fmt.Sprintf("context %s has no AWS profile set", ctxName),
			Hint:   "cml use update " + ctxName + " --profile <profile>",
		}
	}

	paths := awsSharedPaths(deps)
	found, anyFile := awsProfileSet(deps, paths)
	if src, ok := found[profile]; ok {
		return doctorCheck{Check: checkName, Status: doctorOK,
			Detail: fmt.Sprintf("profile %q found in %s", profile, src)}
	}

	c := doctorCheck{
		Check:  checkName,
		Status: doctorFail,
		Detail: fmt.Sprintf("profile %q not found in %s", profile, strings.Join(paths, " or ")),
		Hint:   "aws configure --profile " + profile + " (or 'cml use update " + ctxName + " --profile <other>')",
	}
	if !anyFile {
		c.Detail = fmt.Sprintf("no readable AWS shared files (%s)", strings.Join(paths, ", "))
	}
	return c
}

// sortedProviderContexts returns the context names of one provider, sorted.
func sortedProviderContexts(cfg *config.CMLConfig, provider string) []string {
	var names []string
	for name, ctxCfg := range cfg.Contexts {
		if ctxCfg != nil && ctxCfg.Provider == provider {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// checkOnline verifies the current context's credentials against the cloud
// (STS GetCallerIdentity / a GCP token fetch). Only runs with --online.
func checkOnline(deps doctorDeps, ctx context.Context, cfg *config.CMLConfig) []doctorCheck {
	if deps.awsIdentity == nil && deps.gcpIdentity == nil {
		return nil
	}
	ctxCfg, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok || ctxCfg == nil {
		return nil
	}
	env := errhint.Env{
		Provider: ctxCfg.Provider,
		Context:  cfg.CurrentContext,
		Profile:  ctxCfg.Profile,
		Project:  ctxCfg.Project,
	}

	switch ctxCfg.Provider {
	case "aws":
		c := doctorCheck{Check: "online", Status: doctorOK, Detail: "STS GetCallerIdentity"}
		identity, err := deps.awsIdentity(ctx, ctxCfg.Profile, ctxCfg.Region)
		if err != nil {
			c.Status = doctorFail
			c.Detail = err.Error()
			c.Hint = "aws sso login --profile " + ctxCfg.Profile
			if hints := errhint.Hints(err, env); len(hints) > 0 {
				c.Hint = hints[0]
			}
		} else {
			c.Detail = fmt.Sprintf("STS GetCallerIdentity: %s", identity.Arn)
		}
		return []doctorCheck{c}
	case "gcp":
		if deps.gcpIdentity == nil {
			return nil
		}
		c := doctorCheck{Check: "online", Status: doctorOK, Detail: "GCP token fetch"}
		identity, err := deps.gcpIdentity(ctx, ctxCfg.Project, ctxCfg.Region)
		if err != nil {
			c.Status = doctorFail
			c.Detail = err.Error()
			c.Hint = "gcloud auth application-default login"
			if hints := errhint.Hints(err, env); len(hints) > 0 {
				c.Hint = hints[0]
			}
		} else if identity.Email != "" {
			c.Detail = fmt.Sprintf("GCP token fetch: %s", identity.Email)
		}
		return []doctorCheck{c}
	}
	return nil
}

// doctorGlyph maps a check status to its table symbol and style.
func doctorGlyph(status string) (string, lipgloss.Style) {
	switch status {
	case doctorOK:
		return "✓", ui.RunningStyle
	case doctorWarn:
		return "!", ui.PendingStyle
	default:
		return "✗", ui.StoppedStyle
	}
}

// printDoctorTable renders the ✓/!/✗ table for table mode.
func printDoctorTable(checks []doctorCheck) {
	w := len("CHECK")
	for _, c := range checks {
		if len(c.Check) > w {
			w = len(c.Check)
		}
	}

	fmt.Println()
	fmt.Printf("  %s  %s\n",
		padCtxCol(ui.HeaderStyle.Render("CHECK"), "CHECK", w),
		ui.HeaderStyle.Render("STATUS"))
	fmt.Println(ui.MutedStyle.Render("  " + strings.Repeat("─", w+2+len("STATUS"))))

	ok, warn, fail := 0, 0, 0
	for _, c := range checks {
		glyph, style := doctorGlyph(c.Status)
		fmt.Printf("  %s  %s  %s\n", style.Render(glyph), padCtxCol(c.Check, c.Check, w), c.Detail)
		if c.Hint != "" {
			fmt.Printf("  %s %s\n", ui.HintStyle.Render("hint:"), c.Hint)
		}
		switch c.Status {
		case doctorOK:
			ok++
		case doctorWarn:
			warn++
		default:
			fail++
		}
	}

	fmt.Println()
	fmt.Printf("  %d check(s): %d ok, %d warning(s), %d failed\n", len(checks), ok, warn, fail)
}
