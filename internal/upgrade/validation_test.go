package upgrade

import (
	"strings"
	"testing"
)

// ── ParseMinorParts ───────────────────────────────────────────────────────────

func TestParseMinorParts(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
	}{
		{input: "2.8.5", wantMajor: 2, wantMinor: 8},
		{input: "v2.8.5", wantMajor: 2, wantMinor: 8},
		{input: "2.9.0", wantMajor: 2, wantMinor: 9},
		{input: "3.0.0", wantMajor: 3, wantMinor: 0},
		{input: "2.10.1", wantMajor: 2, wantMinor: 10},
		{input: "1.7", wantMajor: 1, wantMinor: 7},
		{input: "2", wantMajor: 2, wantMinor: 0},
		{input: "", wantMajor: 0, wantMinor: 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseMinorParts(tt.input)
			if got[0] != tt.wantMajor || got[1] != tt.wantMinor {
				t.Errorf("ParseMinorParts(%q) = [%d, %d], want [%d, %d]",
					tt.input, got[0], got[1], tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

// ── ParsePatchPart ────────────────────────────────────────────────────────────

func TestParsePatchPart(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{input: "2.8.5", want: 5},
		{input: "v2.8.6", want: 6},
		{input: "2.9.0", want: 0},
		{input: "2.8.15", want: 15},
		{input: "2.8", want: 0},       // no patch part
		{input: "2", want: 0},         // no patch part
		{input: "", want: 0},          // empty
		{input: "2.8.5-rc1", want: 5}, // ignores suffix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParsePatchPart(tt.input)
			if got != tt.want {
				t.Errorf("ParsePatchPart(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ── ValidatePath ──────────────────────────────────────────────────────────────

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		target      string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid patch upgrade",
			current: "2.8.5",
			target:  "2.8.6",
			wantErr: false,
		},
		{
			name:    "valid minor upgrade",
			current: "2.8.5",
			target:  "2.9.0",
			wantErr: false,
		},
		{
			name:    "valid same version",
			current: "2.8.5",
			target:  "2.8.5",
			wantErr: false,
		},
		{
			name:        "invalid patch downgrade",
			current:     "2.8.6",
			target:      "2.8.5",
			wantErr:     true,
			errContains: "downgrade",
		},
		{
			name:        "invalid minor downgrade",
			current:     "2.9.0",
			target:      "2.8.5",
			wantErr:     true,
			errContains: "downgrade",
		},
		{
			name:        "invalid skip minor version",
			current:     "2.8.5",
			target:      "2.10.0",
			wantErr:     true,
			errContains: "skip minor",
		},
		{
			name:        "invalid skip multiple minors",
			current:     "2.7.0",
			target:      "2.10.0",
			wantErr:     true,
			errContains: "skip",
		},
		{
			name:        "invalid cross-major upgrade",
			current:     "2.8.5",
			target:      "3.0.0",
			wantErr:     true,
			errContains: "cross-major",
		},
		{
			name:    "valid upgrade with v prefix",
			current: "v2.8.5",
			target:  "v2.8.6",
			wantErr: false,
		},
		{
			name:    "valid upgrade mixed v prefix",
			current: "2.8.5",
			target:  "v2.9.0",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.current, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q, %q) error = %v, wantErr %v",
					tt.current, tt.target, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidatePath(%q, %q) error = %v, want error containing %q",
						tt.current, tt.target, err, tt.errContains)
				}
			}
		})
	}
}

// ── Edge cases and regression tests ───────────────────────────────────────────

func TestValidatePath_EdgeCases(t *testing.T) {
	t.Run("upgrade from 2.8.99 to 2.9.0", func(t *testing.T) {
		err := ValidatePath("2.8.99", "2.9.0")
		if err != nil {
			t.Errorf("Should allow upgrade from last patch to next minor: %v", err)
		}
	})

	t.Run("upgrade two minors requires intermediate", func(t *testing.T) {
		err := ValidatePath("2.7.5", "2.9.0")
		if err == nil {
			t.Error("Should require upgrading to 2.8.x first")
		}
		if !strings.Contains(err.Error(), "2.8") {
			t.Errorf("Error should mention intermediate version 2.8: %v", err)
		}
	})

	t.Run("major version 1 to 2", func(t *testing.T) {
		err := ValidatePath("1.6.0", "2.0.0")
		if err == nil {
			t.Error("Should not allow cross-major upgrades")
		}
		if !strings.Contains(err.Error(), "cross-major") {
			t.Errorf("Error should mention cross-major: %v", err)
		}
	})
}
