package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/smithy-go"

	"github.com/vietdv277/cumulus/internal/errhint"
)

// runCaptured executes rootCmd with the given args and returns its combined
// output, restoring the writers and the touched subcommands' SilenceUsage.
func runCaptured(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		if c, _, err := rootCmd.Find(args); err == nil {
			c.SilenceUsage = false
		}
		resetLegacyFlags()
	})

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestRuntimeErrorOmitsUsage(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	out, err := runCaptured(t, "alias", "rm", "nosuch")
	if err == nil {
		t.Fatal("expected an error removing an unknown alias")
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("runtime error printed usage:\n%s", out)
	}
	if strings.Contains(out, err.Error()) {
		t.Errorf("cobra printed the error itself; Execute prints it once:\n%s", out)
	}
}

func TestArgErrorKeepsUsage(t *testing.T) {
	setTestConfig(t, testConfigYAML)

	out, err := runCaptured(t, "alias", "add", "onlyone")
	if err == nil {
		t.Fatal("expected an argument error")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("argument error should print usage, got:\n%s", out)
	}
}

func TestPrintError(t *testing.T) {
	t.Run("with hints", func(t *testing.T) {
		err := fmt.Errorf("list instances: %w", &smithy.OperationError{
			ServiceID: "EC2", OperationName: "DescribeInstances",
			Err: &smithy.GenericAPIError{
				Code: "UnauthorizedOperation",
				Message: "You are not authorized to perform this operation. " +
					"User: arn:aws:iam::123456789012:user/dev is not authorized to perform: ec2:DescribeInstances",
			},
		})

		var buf bytes.Buffer
		printError(&buf, err, errhint.Env{Provider: "aws", Context: "aws:prod", Profile: "dev"})

		want := "Error: list instances: operation error EC2: DescribeInstances, " +
			"api error UnauthorizedOperation: You are not authorized to perform this operation. " +
			"User: arn:aws:iam::123456789012:user/dev is not authorized to perform: ec2:DescribeInstances\n" +
			"Hint: profile \"dev\" is not allowed to call ec2:DescribeInstances — check its IAM policy or use another context\n"
		if buf.String() != want {
			t.Fatalf("printError wrote:\n%q\nwant:\n%q", buf.String(), want)
		}
	})

	t.Run("without hints", func(t *testing.T) {
		var buf bytes.Buffer
		printError(&buf, errors.New("boom"), errhint.Env{})

		if got, want := buf.String(), "Error: boom\n"; got != want {
			t.Fatalf("printError wrote %q, want %q", got, want)
		}
	})
}

func TestHintEnv(t *testing.T) {
	t.Run("legacy command without a context flag uses the profile", func(t *testing.T) {
		setTestConfig(t, testConfigYAML)

		old := profile
		profile = "legacy"
		t.Cleanup(func() { profile = old })

		// rootCmd has no --context flag, so even with a config present the
		// legacy profile stands.
		if env := hintEnv(rootCmd); env != (errhint.Env{Profile: "legacy"}) {
			t.Fatalf("hintEnv(rootCmd) = %+v, want the legacy profile", env)
		}
	})

	t.Run("context flag resolves the current context", func(t *testing.T) {
		setTestConfig(t, testConfigYAML)

		env := hintEnv(vmCmd)
		want := errhint.Env{Provider: "aws", Context: "aws:prod", Profile: "prod-sso"}
		if env != want {
			t.Fatalf("hintEnv(vmCmd) = %+v, want %+v", env, want)
		}
	})

	t.Run("context flag resolves the flag value", func(t *testing.T) {
		setTestConfig(t, testConfigYAML)

		if err := vmCmd.PersistentFlags().Set("context", "gcp:staging"); err != nil {
			t.Fatalf("set context flag: %v", err)
		}
		t.Cleanup(func() { _ = vmCmd.PersistentFlags().Set("context", "") })

		env := hintEnv(vmCmd)
		want := errhint.Env{Provider: "gcp", Context: "gcp:staging", Project: "my-project"}
		if env != want {
			t.Fatalf("hintEnv(vmCmd) = %+v, want %+v", env, want)
		}
	})

	t.Run("unresolvable context falls back to the profile", func(t *testing.T) {
		setTestConfig(t, testConfigYAML)

		old := profile
		profile = "legacy"
		t.Cleanup(func() { profile = old })

		if err := vmCmd.PersistentFlags().Set("context", "nosuch"); err != nil {
			t.Fatalf("set context flag: %v", err)
		}
		t.Cleanup(func() { _ = vmCmd.PersistentFlags().Set("context", "") })

		if env := hintEnv(vmCmd); env != (errhint.Env{Profile: "legacy"}) {
			t.Fatalf("hintEnv(vmCmd) = %+v, want the legacy profile", env)
		}
	})
}
