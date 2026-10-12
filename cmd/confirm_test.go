package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"YES with surrounding spaces", "  YES \n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"empty line", "\n", false},
		{"garbage", "maybe\n", false},
		{"EOF without newline", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := confirm(strings.NewReader(tt.input), &out, "Proceed? [y/N]")
			if err != nil {
				t.Fatalf("confirm(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("confirm(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if !strings.HasPrefix(out.String(), "Proceed? [y/N]") {
				t.Fatalf("prompt not written, out = %q", out.String())
			}
		})
	}
}

func TestConfirmAction(t *testing.T) {
	refusal := "refusing to stop i-0abc without confirmation; pass --yes"

	t.Run("approved skips the prompt entirely", func(t *testing.T) {
		proceed, err := confirmAction(true, strings.NewReader("n\n"), &bytes.Buffer{}, "Proceed? [y/N]", refusal)
		if err != nil || !proceed {
			t.Fatalf("confirmAction() = %v, %v; want true, nil", proceed, err)
		}
	})

	t.Run("non-terminal stdin refuses", func(t *testing.T) {
		orig := stdinIsTerminal
		stdinIsTerminal = func() bool { return false }
		t.Cleanup(func() { stdinIsTerminal = orig })

		proceed, err := confirmAction(false, strings.NewReader("y\n"), &bytes.Buffer{}, "Proceed? [y/N]", refusal)
		if proceed {
			t.Fatal("confirmAction() = true, want refusal to block the action")
		}
		if err == nil || err.Error() != refusal {
			t.Fatalf("confirmAction() error = %v, want %q", err, refusal)
		}
	})

	t.Run("terminal stdin prompts and yes proceeds", func(t *testing.T) {
		orig := stdinIsTerminal
		stdinIsTerminal = func() bool { return true }
		t.Cleanup(func() { stdinIsTerminal = orig })

		proceed, err := confirmAction(false, strings.NewReader("y\n"), &bytes.Buffer{}, "Proceed? [y/N]", refusal)
		if err != nil || !proceed {
			t.Fatalf("confirmAction() = %v, %v; want true, nil", proceed, err)
		}
	})

	t.Run("terminal stdin prompts and no declines", func(t *testing.T) {
		orig := stdinIsTerminal
		stdinIsTerminal = func() bool { return true }
		t.Cleanup(func() { stdinIsTerminal = orig })

		proceed, err := confirmAction(false, strings.NewReader("n\n"), &bytes.Buffer{}, "Proceed? [y/N]", refusal)
		if err != nil || proceed {
			t.Fatalf("confirmAction() = %v, %v; want false, nil", proceed, err)
		}
	})
}

func TestIsSelectorCancelled(t *testing.T) {
	if isSelectorCancelled(nil) {
		t.Fatal("isSelectorCancelled(nil) = true")
	}
	if !isSelectorCancelled(errors.New("selection cancelled")) {
		t.Fatal("isSelectorCancelled(selection cancelled) = false")
	}
	if isSelectorCancelled(errors.New("error running selector: boom")) {
		t.Fatal("real selector error treated as cancellation")
	}
}
