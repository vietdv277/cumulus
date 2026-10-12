package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/vietdv277/cumulus/internal/config"
	"github.com/vietdv277/cumulus/internal/ui"
	"github.com/vietdv277/cumulus/pkg/provider"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Manage saved port-forwarding tunnels",
	Long: `Manage saved tunnel configurations.

Tunnels are saved with 'cml vm tunnel <vm> <remote-port> [local-port] --save <name>'
and stored in the config file under 'tunnels:'. Starting a saved tunnel always
uses the context it was saved with, not the current one.

Examples:
  cml tunnel list                        # List saved tunnels
  cml tunnel start db-prod               # Start a saved tunnel
  cml tunnel start db-prod --local-port 25432
  cml tunnel rm db-prod                  # Remove a saved tunnel`,
}

var tunnelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved tunnels",
	RunE:  runTunnelList,
}

var tunnelStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a saved tunnel",
	Long: `Start a saved tunnel, using the context it was saved with.

Examples:
  cml tunnel start db-prod
  cml tunnel start db-prod --local-port 25432`,
	Args: cobra.ExactArgs(1),
	RunE: runTunnelStart,
}

var tunnelRmCmd = &cobra.Command{
	Use:     "rm <name>",
	Aliases: []string{"remove"},
	Short:   "Remove a saved tunnel",
	Args:    cobra.ExactArgs(1),
	RunE:    runTunnelRm,
}

var tunnelStartLocalPort int

func init() {
	rootCmd.AddCommand(tunnelCmd)
	tunnelCmd.AddCommand(tunnelListCmd)
	tunnelCmd.AddCommand(tunnelStartCmd)
	tunnelCmd.AddCommand(tunnelRmCmd)

	tunnelStartCmd.Flags().IntVar(&tunnelStartLocalPort, "local-port", 0, "Local port (default: the saved local port)")
}

// tunnelEntry is the stable json/yaml shape of a saved tunnel.
type tunnelEntry struct {
	Name       string `json:"name" yaml:"name"`
	Context    string `json:"context" yaml:"context"`
	Target     string `json:"target" yaml:"target"`
	RemotePort int    `json:"remote_port" yaml:"remote_port"`
	LocalPort  int    `json:"local_port" yaml:"local_port"`
}

func runTunnelList(cmd *cobra.Command, args []string) error {
	out, err := outputFormat(cmd)
	if err != nil {
		return err
	}

	names, tunnels, err := config.ListTunnels()
	if err != nil {
		return fmt.Errorf("failed to list tunnels: %w", err)
	}

	if len(names) == 0 && out == OutputTable {
		fmt.Println("No saved tunnels. Save one with 'cml vm tunnel <vm> <remote-port> --save <name>'.")
		return nil
	}

	entries := make([]tunnelEntry, 0, len(names))
	for _, name := range names {
		t := tunnels[name]
		entries = append(entries, tunnelEntry{
			Name:       name,
			Context:    t.Context,
			Target:     t.Target,
			RemotePort: t.RemotePort,
			LocalPort:  t.LocalPort,
		})
	}

	return render(os.Stdout, out, entries, func() { printTunnelTable(entries) })
}

// tunnelProviderFactory builds the VM provider for a saved tunnel's context.
// Package var so tests can inject a fake provider.
var tunnelProviderFactory = buildVMProvider

func runTunnelStart(cmd *cobra.Command, args []string) error {
	tunnel, err := config.GetTunnelConfig(args[0])
	if err != nil {
		return err
	}

	if tunnel.Bastion != "" || tunnel.RemoteHost != "" {
		return fmt.Errorf("tunnel %q forwards via a bastion/remote host — not supported yet", args[0])
	}

	// Always use the tunnel's own context, never the current one.
	if tunnel.Context == "" {
		return fmt.Errorf("tunnel %q has no saved context", args[0])
	}
	ctxConfig, ctxName, err := config.ResolveContext(tunnel.Context)
	if err != nil {
		return err
	}

	localPort := tunnel.LocalPort
	if cmd.Flags().Changed("local-port") {
		localPort = tunnelStartLocalPort
	}
	if localPort == 0 {
		localPort = tunnel.RemotePort
	}

	ctx := context.Background()
	vmProvider, err := tunnelProviderFactory(ctx, ctxConfig, ctxName)
	if err != nil {
		return err
	}

	opts := &provider.TunnelOptions{
		LocalPort:  localPort,
		RemotePort: tunnel.RemotePort,
	}

	fmt.Printf("Creating tunnel %s to %s: localhost:%d -> remote:%d\n", args[0], tunnel.Target, localPort, tunnel.RemotePort)
	fmt.Println("Press Ctrl+C to close the tunnel")

	return vmProvider.Tunnel(ctx, tunnel.Target, opts)
}

func runTunnelRm(cmd *cobra.Command, args []string) error {
	if err := config.DeleteTunnel(args[0]); err != nil {
		return err
	}
	fmt.Printf("Tunnel removed: %s\n", args[0])
	return nil
}

// printTunnelTable prints saved tunnels in table mode, sorted by name.
func printTunnelTable(entries []tunnelEntry) {
	w0 := runewidth.StringWidth("NAME")
	w1 := runewidth.StringWidth("CONTEXT")
	w2 := runewidth.StringWidth("TARGET")
	w3 := runewidth.StringWidth("LOCAL:REMOTE")
	for _, e := range entries {
		w0 = max(w0, runewidth.StringWidth(e.Name))
		w1 = max(w1, runewidth.StringWidth(e.Context))
		w2 = max(w2, runewidth.StringWidth(e.Target))
		w3 = max(w3, runewidth.StringWidth(tunnelPorts(e)))
	}

	fmt.Println()
	fmt.Printf("  %s  %s  %s  %s\n",
		ui.HeaderStyle.Render(padTunnelCol("NAME", w0)),
		ui.HeaderStyle.Render(padTunnelCol("CONTEXT", w1)),
		ui.HeaderStyle.Render(padTunnelCol("TARGET", w2)),
		ui.HeaderStyle.Render(padTunnelCol("LOCAL:REMOTE", w3)))
	fmt.Println(ui.MutedStyle.Render("  " + strings.Repeat("─", w0+2+w1+2+w2+2+w3)))

	for _, e := range entries {
		fmt.Printf("  %s  %s  %s  %s\n",
			padTunnelCol(e.Name, w0),
			padTunnelCol(e.Context, w1),
			padTunnelCol(e.Target, w2),
			tunnelPorts(e))
	}

	fmt.Println()
	fmt.Printf("  %d saved tunnels\n", len(entries))
}

func tunnelPorts(e tunnelEntry) string {
	return fmt.Sprintf("%d:%d", e.LocalPort, e.RemotePort)
}

// padTunnelCol pads a plain string to the given display width.
func padTunnelCol(s string, width int) string {
	pw := runewidth.StringWidth(s)
	if pw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-pw)
}
