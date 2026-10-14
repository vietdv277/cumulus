package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/gcp"
	"github.com/vietdv277/cumulus/internal/suggest"
	"github.com/vietdv277/cumulus/pkg/provider"
	"github.com/vietdv277/cumulus/pkg/types"
)

var pamCmd = &cobra.Command{
	Use:   "pam",
	Short: "Request temporary access with GCP Privileged Access Manager",
	Long: `Request just-in-time access through GCP Privileged Access Manager (PAM)
entitlements in the context project. GCP contexts only.

Examples:
  cml pam list                                   # entitlements available to request
  cml pam request prod-admin -j "INC-123 hotfix" # 1h grant
  cml pam request prod-admin -d 4h -j "release" --wait
  cml pam grants                                 # open grants
  cml pam grants --all                           # including ended ones`,
}

var pamListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List entitlements available to request",
	Args:    cobra.NoArgs,
	RunE:    runPAMList,
}

var pamRequestCmd = &cobra.Command{
	Use:   "request <entitlement>",
	Short: "Request a grant on an entitlement",
	Long: `Request a grant on an entitlement for --duration (default 1h, at most the
entitlement's maximum). An entitlement that requires a justification prompts
for one when -j is not given on a terminal.

When an open grant (active or awaiting approval) already exists on the
entitlement, it is shown and no new request is made.

A grant that needs no approval activates within seconds; request waits for
it (up to 2 minutes). --wait also waits through approval, however long it
takes, and fails when the grant is denied or its activation fails. Ctrl-C
stops waiting; the request stays open.`,
	Args: cobra.ExactArgs(1),
	RunE: runPAMRequest,
}

var pamGrantsCmd = &cobra.Command{
	Use:   "grants",
	Short: "List grants requested by the caller",
	Args:  cobra.NoArgs,
	RunE:  runPAMGrants,
}

var (
	pamContextFlag   string
	pamLocation      string
	pamDuration      time.Duration
	pamJustification string
	pamWait          bool
	pamAll           bool
)

func init() {
	rootCmd.AddCommand(pamCmd)
	pamCmd.AddCommand(pamListCmd, pamRequestCmd, pamGrantsCmd)

	pamCmd.PersistentFlags().StringVarP(&pamContextFlag, "context", "c", "", "Use specific context")
	pamCmd.PersistentFlags().StringVar(&pamLocation, "location", gcp.DefaultPAMLocation, "PAM location of the entitlements")

	pamRequestCmd.Flags().DurationVarP(&pamDuration, "duration", "d", time.Hour, "How long the access lasts (e.g. 30m, 2h)")
	pamRequestCmd.Flags().StringVarP(&pamJustification, "justification", "j", "", "Reason for the request")
	pamRequestCmd.Flags().BoolVarP(&pamWait, "wait", "w", false, "Also wait through approval until the grant is active")

	pamGrantsCmd.Flags().BoolVarP(&pamAll, "all", "a", false, "Include ended, denied and expired grants")
}

// newGCPPAMProvider builds the PAM provider; a variable so tests can inject a fake.
var newGCPPAMProvider = func(ctx context.Context, ctxConfig *config.Context, location string) (provider.PAMProvider, error) {
	client, err := gcp.NewClient(ctx,
		gcp.WithProject(ctxConfig.Project),
		gcp.WithRegion(ctxConfig.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP client: %w", err)
	}
	return gcp.NewPAMProvider(client, location), nil
}

func getPAMProvider(ctx context.Context) (provider.PAMProvider, error) {
	ctxConfig, ctxName, err := resolveContext(pamContextFlag)
	if err != nil {
		return nil, err
	}
	if ctxConfig.Provider != "gcp" {
		return nil, fmt.Errorf("pam needs a GCP context (Privileged Access Manager); %s is %s", ctxName, ctxConfig.Provider)
	}
	return newGCPPAMProvider(ctx, ctxConfig, pamLocation)
}

func runPAMList(cmd *cobra.Command, _ []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := getPAMProvider(ctx)
	if err != nil {
		return err
	}
	ents, err := p.ListEntitlements(ctx)
	if err != nil {
		return err
	}
	if len(ents) == 0 && out == OutputTable {
		fmt.Println("No entitlements available to request")
		return nil
	}
	return render(os.Stdout, out, ents, func() { printEntitlements(ents) })
}

func runPAMGrants(cmd *cobra.Command, _ []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := getPAMProvider(ctx)
	if err != nil {
		return err
	}
	grants, err := p.ListGrants(ctx)
	if err != nil {
		return err
	}
	if !pamAll {
		grants = openGrants(grants)
	}
	if len(grants) == 0 && out == OutputTable {
		if pamAll {
			fmt.Println("No grants found")
		} else {
			fmt.Println("No open grants (use --all to include ended ones)")
		}
		return nil
	}
	return render(os.Stdout, out, grants, func() { printGrants(grants) })
}

func runPAMRequest(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	p, err := getPAMProvider(ctx)
	if err != nil {
		return err
	}
	// Progress goes to stderr so -o json/yaml keeps stdout to the document.
	r := &pamRequester{p: p, in: os.Stdin, status: os.Stderr, sleep: sleepContext,
		interval: pamPollInterval, activationTimeout: pamActivationTimeout}
	grant, err := r.request(ctx, args[0], pamDuration, pamJustification, pamWait)
	if err != nil {
		return err
	}
	return render(os.Stdout, out, grant, func() { printGrantSummary(*grant) })
}

// pamPollInterval is how often a pending grant is re-read while it awaits
// approval; activation takes seconds, so a scheduled grant is polled faster.
const (
	pamPollInterval       = 5 * time.Second
	pamActivationInterval = 2 * time.Second
	// pamActivationTimeout bounds the wait for activation without --wait.
	pamActivationTimeout = 2 * time.Minute
)

// pamRequester runs a grant request: validation, reuse of an open grant, and
// the optional wait. Its I/O and clock are injectable for tests.
type pamRequester struct {
	p        provider.PAMProvider
	in       io.Reader
	status   io.Writer
	sleep    func(context.Context, time.Duration) error
	interval time.Duration
	// activationTimeout bounds the default wait for a grant that needs no
	// approval; 0 means no default wait.
	activationTimeout time.Duration
}

func (r *pamRequester) request(ctx context.Context, id string, d time.Duration, justification string, wait bool) (*types.Grant, error) {
	ents, err := r.p.ListEntitlements(ctx)
	if err != nil {
		return nil, err
	}
	ent, err := findEntitlement(ents, id)
	if err != nil {
		return nil, err
	}
	if d <= 0 {
		return nil, fmt.Errorf("--duration must be positive")
	}
	if ent.MaxDuration > 0 && d > ent.MaxDuration {
		return nil, fmt.Errorf("--duration %s exceeds the %s maximum of entitlement %s", shortDuration(d), shortDuration(ent.MaxDuration), ent.ID)
	}

	grants, err := r.p.ListGrants(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if g.Entitlement == ent.ID && g.Open() {
			_, _ = fmt.Fprintf(r.status, "An open grant already exists on %s; no new request made.\n", ent.ID)
			return r.wait(ctx, &g, wait)
		}
	}

	justification = strings.TrimSpace(justification)
	if justification == "" && ent.JustificationRequired {
		if !stdinIsTerminal() {
			return nil, fmt.Errorf("entitlement %s requires a justification; pass -j <reason>", ent.ID)
		}
		if justification, err = promptLine(r.in, r.status, "Justification:"); err != nil {
			return nil, err
		}
		if justification == "" {
			return nil, fmt.Errorf("entitlement %s requires a justification", ent.ID)
		}
	}

	grant, err := r.p.RequestGrant(ctx, ent.Name, provider.GrantRequest{Duration: d, Justification: justification})
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(r.status, "Requested %s on %s for %s: %s\n", grant.ID, ent.ID, shortDuration(d), grant.State)
	return r.wait(ctx, grant, wait)
}

// wait polls a pending grant until it is active or has failed. With --wait
// it waits as long as it takes; without it, it waits only for a grant that
// needs no approval (SCHEDULED/ACTIVATING), for at most activationTimeout.
// Ctrl-C or the timeout ends the wait without an error, returning the last state.
func (r *pamRequester) wait(ctx context.Context, g *types.Grant, explicit bool) (*types.Grant, error) {
	if !explicit {
		if g.State == "APPROVAL_AWAITED" || r.activationTimeout <= 0 {
			return g, nil
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.activationTimeout)
		defer cancel()
	}
	last := g.State
	for g.Pending() {
		interval := r.interval
		if g.State != "APPROVAL_AWAITED" {
			interval = min(interval, pamActivationInterval)
		}
		if err := r.sleep(ctx, interval); err != nil {
			_, _ = fmt.Fprintf(r.status, "Stopped waiting; grant %s is still %s.\n", g.ID, g.State)
			return g, nil
		}
		next, err := r.p.GetGrant(ctx, g.Name)
		if err != nil {
			if ctx.Err() != nil {
				_, _ = fmt.Fprintf(r.status, "Stopped waiting; grant %s is still %s.\n", g.ID, g.State)
				return g, nil
			}
			return nil, err
		}
		g = next
		if g.State != last {
			_, _ = fmt.Fprintf(r.status, "Grant %s: %s\n", g.ID, g.State)
			last = g.State
		}
	}
	if !g.Open() {
		return nil, fmt.Errorf("grant %s on %s ended as %s", g.ID, g.Entitlement, g.State)
	}
	return g, nil
}

// findEntitlement matches id against entitlement IDs or full names.
func findEntitlement(ents []types.Entitlement, id string) (*types.Entitlement, error) {
	ids := make([]string, 0, len(ents))
	for i := range ents {
		if ents[i].ID == id || ents[i].Name == id {
			return &ents[i], nil
		}
		ids = append(ids, ents[i].ID)
	}
	if hint := suggest.DidYouMean(suggest.Similar(id, ids, 3)); hint != "" {
		return nil, fmt.Errorf("no requestable entitlement %q — %s", id, hint)
	}
	return nil, fmt.Errorf("no requestable entitlement %q — see 'cml pam list'", id)
}

func openGrants(grants []types.Grant) []types.Grant {
	var open []types.Grant
	for _, g := range grants {
		if g.Open() {
			open = append(open, g)
		}
	}
	return open
}

// promptLine asks for one line of input on the terminal.
func promptLine(in io.Reader, out io.Writer, prompt string) (string, error) {
	_, _ = fmt.Fprint(out, prompt+" ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read input: %w", err)
		}
		return "", nil
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// sleepContext waits d, returning early when ctx is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// shortDuration formats d without zero units, e.g. 1h, 1h30m, 45m.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// rolesSummary shows the first role and how many more there are.
func rolesSummary(roles []string) string {
	switch len(roles) {
	case 0:
		return "-"
	case 1:
		return displayRole(roles[0])
	default:
		return fmt.Sprintf("%s +%d", displayRole(roles[0]), len(roles)-1)
	}
}

// displayRole shortens a custom role ("organizations/<id>/roles/x" or
// "projects/<p>/roles/x") to "custom/x"; predefined roles stay as they are.
func displayRole(role string) string {
	if strings.HasPrefix(role, "roles/") {
		return role
	}
	if i := strings.Index(role, "/roles/"); i >= 0 {
		return "custom/" + role[i+len("/roles/"):]
	}
	return role
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func printEntitlements(ents []types.Entitlement) {
	headers := []string{"Entitlement", "Roles", "Max", "Approval", "Reason"}
	widths := []int{35, 32, 5, 8, 8}
	rows := make([][]string, 0, len(ents))
	for _, e := range ents {
		just := "optional"
		if e.JustificationRequired {
			just = "required"
		}
		rows = append(rows, []string{e.ID, rolesSummary(e.Roles), shortDuration(e.MaxDuration), yesNo(e.NeedsApproval), just})
	}
	renderSimpleTable(headers, widths, rows)
	fmt.Printf("  %d entitlements\n", len(ents))
}

func printGrants(grants []types.Grant) {
	// Grant IDs are UUIDs; the table shows a prefix, -o json the full ID.
	headers := []string{"Grant", "Entitlement", "State", "Duration", "Expires"}
	widths := []int{8, 35, 16, 8, 16}
	rows := make([][]string, 0, len(grants))
	for _, g := range grants {
		rows = append(rows, []string{shortID(g.ID), g.Entitlement, g.State, shortDuration(g.Duration), localTime(g.ExpiresAt)})
	}
	renderSimpleTable(headers, widths, rows)
	fmt.Printf("  %d grants\n", len(grants))
}

func printGrantSummary(g types.Grant) {
	fmt.Printf("Grant:       %s\n", g.ID)
	fmt.Printf("Entitlement: %s\n", g.Entitlement)
	fmt.Printf("State:       %s\n", g.State)
	fmt.Printf("Duration:    %s\n", shortDuration(g.Duration))
	if !g.ExpiresAt.IsZero() {
		fmt.Printf("Expires:     %s\n", localTime(g.ExpiresAt))
	}
	switch g.State {
	case "APPROVAL_AWAITED":
		fmt.Println("Waiting for an approver; check with 'cml pam grants' or rerun with --wait.")
	case "SCHEDULED", "ACTIVATING":
		fmt.Println("Activation in progress; check with 'cml pam grants'.")
	case "ACTIVE":
		fmt.Println("IAM changes can take a minute to apply.")
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func localTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}
