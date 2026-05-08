package deployment

import (
	"testing"
)

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name        string
		flag        string
		wantMode    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "explicit k3s",
			flag:     "k3s",
			wantMode: "k3s",
			wantErr:  false,
		},
		{
			name:     "explicit k3d",
			flag:     "k3d",
			wantMode: "k3d",
			wantErr:  false,
		},
		{
			name:     "case insensitive K3S",
			flag:     "K3S",
			wantMode: "k3s",
			wantErr:  false,
		},
		{
			name:     "case insensitive K3D",
			flag:     "K3D",
			wantMode: "k3d",
			wantErr:  false,
		},
		{
			name:     "empty flag auto-detects",
			flag:     "",
			wantMode: "", // will be either k3s or k3d depending on environment
			wantErr:  false,
		},
		{
			name:        "invalid mode",
			flag:        "docker",
			wantMode:    "",
			wantErr:     true,
			errContains: "invalid mode",
		},
		{
			name:        "invalid mode typo",
			flag:        "k3",
			wantMode:    "",
			wantErr:     true,
			errContains: "invalid mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, reason, err := ResolveMode(tt.flag, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveMode(%q) error = %v, wantErr %v", tt.flag, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ResolveMode(%q) error = %v, want error containing %q", tt.flag, err, tt.errContains)
				}
				return
			}

			// For explicit modes, check exact match
			if tt.flag != "" && tt.wantMode != "" {
				if mode != tt.wantMode {
					t.Errorf("ResolveMode(%q) mode = %q, want %q", tt.flag, mode, tt.wantMode)
				}
			}

			// For auto-detect, just verify we got a valid mode
			if tt.flag == "" {
				if mode != "k3s" && mode != "k3d" {
					t.Errorf("ResolveMode(\"\") auto-detected mode = %q, want either k3s or k3d", mode)
				}
				if reason == "" {
					t.Error("ResolveMode(\"\") should return a reason for auto-detection")
				}
			}

			// Explicit modes should have empty reason
			if tt.flag != "" && !tt.wantErr && reason != "" {
				t.Errorf("ResolveMode(%q) returned unexpected reason: %q", tt.flag, reason)
			}
		})
	}
}

// Test that auto-detection returns consistent results
func TestResolveMode_AutoDetectConsistency(t *testing.T) {
	mode1, reason1, err1 := ResolveMode("", false)
	if err1 != nil {
		t.Fatalf("First auto-detect failed: %v", err1)
	}

	mode2, reason2, err2 := ResolveMode("", false)
	if err2 != nil {
		t.Fatalf("Second auto-detect failed: %v", err2)
	}

	if mode1 != mode2 {
		t.Errorf("Auto-detect not consistent: first=%q, second=%q", mode1, mode2)
	}

	if reason1 != reason2 {
		t.Errorf("Auto-detect reason not consistent: first=%q, second=%q", reason1, reason2)
	}
}

// Test helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}
