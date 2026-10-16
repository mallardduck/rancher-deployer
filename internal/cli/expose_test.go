package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mallardduck/rancher-deployer/internal/expose"
)

func TestRunExposeSteps(t *testing.T) {
	calls := 0
	flaky := expose.Step{Title: "flaky", Check: func(context.Context) error {
		calls++
		if calls < 2 {
			return errors.New("not yet")
		}
		return nil
	}}
	failing := expose.Step{Title: "failing", Check: func(context.Context) error { return errors.New("nope") }}

	t.Run("no steps", func(t *testing.T) {
		if err := RunExposeSteps(context.Background(), nil, false, strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("retry then pass", func(t *testing.T) {
		calls = 0
		// Enter to acknowledge, Enter to accept the default "check again".
		if err := RunExposeSteps(context.Background(), []expose.Step{flaky}, false, strings.NewReader("\n\n")); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Errorf("Check called %d times, want 2", calls)
		}
	})
	t.Run("decline retry aborts", func(t *testing.T) {
		if err := RunExposeSteps(context.Background(), []expose.Step{failing}, false, strings.NewReader("\nn\n")); err == nil {
			t.Error("want error when the user declines to retry")
		}
	})
	t.Run("yes skips waits but fails a bad check", func(t *testing.T) {
		if err := RunExposeSteps(context.Background(), []expose.Step{failing}, true, strings.NewReader("")); err == nil {
			t.Error("want error under --yes when Check fails")
		}
	})
}
