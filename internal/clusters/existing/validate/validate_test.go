package validate

import (
	"strings"
	"testing"
)

func TestExtractMinor(t *testing.T) {
	for in, want := range map[string]string{
		"1.28.10": "1.28",
		"1.28":    "1.28",
		"1":       "1",
	} {
		if got := extractMinor(in); got != want {
			t.Errorf("extractMinor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVersion(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
		wantErr  bool
	}{
		{"same minor", "v1.28.10", "v1.28.4", false},
		{"v prefix ignored", "1.28.10", "v1.28", false},
		{"different minor", "v1.27.3", "v1.28.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Version(tt.cluster, tt.expected)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "mismatch") {
				t.Errorf("error should mention mismatch, got %v", err)
			}
		})
	}
}
