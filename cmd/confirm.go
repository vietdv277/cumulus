package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vietdv277/cumulus/internal/config"
)

// confirm asks a yes/no question and returns whether the user answered yes.
// Only y/yes (case-insensitive) count as yes; empty input, n/no, unrecognized
// input and EOF all default to no.
func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	_, _ = fmt.Fprint(out, prompt+" ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		_, _ = fmt.Fprintln(out)
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// stdinIsTerminal reports whether stdin is an interactive terminal. It is a
// variable so tests can inject the check.
var stdinIsTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// confirmAction decides whether a destructive action may proceed. approved is
// the --yes flag value; when it is false and stdin is not a terminal, it fails
// with refusal instead of hanging on a prompt that cannot be answered.
func confirmAction(approved bool, in io.Reader, out io.Writer, prompt, refusal string) (bool, error) {
	if approved {
		return true, nil
	}
	if !stdinIsTerminal() {
		return false, errors.New(refusal)
	}
	ok, err := confirm(in, out, prompt)
	if err != nil || !ok {
		return false, err
	}
	return true, nil
}

// contextNameForPrompt returns the context name shown in confirmation prompts:
// the --context override when set, else the active context.
func contextNameForPrompt(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	_, name, err := config.GetCurrentContext()
	if err != nil {
		return ""
	}
	return name
}

// vmTargetLabel formats a VM for confirmation prompts, e.g. "web (i-0abc123)".
func vmTargetLabel(name, id string) string {
	if name == "" || name == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// isSelectorCancelled reports whether err is a user cancellation (Esc/q/Ctrl-C)
// from the interactive selectors in internal/ui, as opposed to a real failure.
func isSelectorCancelled(err error) bool {
	return err != nil && err.Error() == "selection cancelled"
}
