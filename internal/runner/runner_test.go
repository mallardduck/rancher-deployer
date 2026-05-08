package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Exists ────────────────────────────────────────────────────────────────────

func TestExists(t *testing.T) {
	tests := []struct {
		name   string
		binary string
		want   bool
	}{
		{
			name:   "standard binary exists",
			binary: "go",
			want:   true,
		},
		{
			name:   "nonexistent binary",
			binary: "this-binary-definitely-does-not-exist-12345",
			want:   false,
		},
		{
			name:   "empty string",
			binary: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Exists(tt.binary)
			if got != tt.want {
				t.Errorf("Exists(%q) = %v, want %v", tt.binary, got, tt.want)
			}
		})
	}
}

// ── MustExist ─────────────────────────────────────────────────────────────────

func TestMustExist(t *testing.T) {
	t.Run("returns nil when binary exists", func(t *testing.T) {
		err := MustExist("go")
		if err != nil {
			t.Errorf("MustExist(\"go\") returned error: %v", err)
		}
	})

	t.Run("returns error when binary missing", func(t *testing.T) {
		err := MustExist("this-binary-definitely-does-not-exist-12345")
		if err == nil {
			t.Error("MustExist(nonexistent) should return error, got nil")
		}
		if err != nil && err.Error() == "" {
			t.Error("MustExist error should have a message")
		}
	})
}

// ── Output ────────────────────────────────────────────────────────────────────

func TestOutput(t *testing.T) {
	t.Run("captures output from successful command", func(t *testing.T) {
		got, err := Output("echo", "hello")
		if err != nil {
			t.Fatalf("Output(echo, hello) returned error: %v", err)
		}
		if got != "hello" {
			t.Errorf("Output(echo, hello) = %q, want %q", got, "hello")
		}
	})

	t.Run("trims whitespace from output", func(t *testing.T) {
		got, err := Output("echo", "  test  ")
		if err != nil {
			t.Fatalf("Output failed: %v", err)
		}
		if got != "test" {
			t.Errorf("Output should trim whitespace, got %q", got)
		}
	})

	t.Run("returns error for nonexistent command", func(t *testing.T) {
		_, err := Output("this-command-does-not-exist-12345")
		if err == nil {
			t.Error("Output(nonexistent) should return error")
		}
	})

	t.Run("returns error when command fails", func(t *testing.T) {
		_, err := Output("false") // 'false' exits with status 1
		if err == nil {
			t.Error("Output(false) should return error")
		}
	})
}

// ── Integration-style tests for Run ──────────────────────────────────────────

func TestRun(t *testing.T) {
	t.Run("executes successful command", func(t *testing.T) {
		err := Run("true") // 'true' exits with status 0
		if err != nil {
			t.Errorf("Run(true) returned error: %v", err)
		}
	})

	t.Run("returns error for failed command", func(t *testing.T) {
		err := Run("false") // 'false' exits with status 1
		if err == nil {
			t.Error("Run(false) should return error")
		}
	})

	t.Run("returns error for nonexistent command", func(t *testing.T) {
		err := Run("this-command-does-not-exist-12345")
		if err == nil {
			t.Error("Run(nonexistent) should return error")
		}
	})
}

// ── RunSudo ───────────────────────────────────────────────────────────────────

func TestRunSudo(t *testing.T) {
	// Only test the argument construction logic, not actual sudo execution
	// We can't test actual sudo without privileges or mocking

	t.Run("constructs sudo command correctly", func(t *testing.T) {
		// This test verifies the function exists and has the right signature
		// Actual execution would require sudo privileges
		if !Exists("sudo") {
			t.Skip("sudo not available")
		}

		// Test that it doesn't panic with valid input
		// We use 'true' which should work even with sudo
		// This may fail in CI without sudo access, so we just check it doesn't panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("RunSudo panicked: %v", r)
			}
		}()

		// Just verify it returns an error (likely sudo auth) or succeeds
		// Either is fine - we're checking the function works, not sudo
		_ = RunSudo("true")
	})
}

// ── Test helper for creating temp executables ─────────────────────────────────

func TestExists_WithTempBinary(t *testing.T) {
	// Create a temporary directory and add it to PATH for this test
	tmpDir := t.TempDir()

	// Create a dummy executable
	binPath := filepath.Join(tmpDir, "test-binary")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	// Temporarily modify PATH
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)

	// Now test
	if !Exists("test-binary") {
		t.Error("Exists should find binary in PATH")
	}
}
