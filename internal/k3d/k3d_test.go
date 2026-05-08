package k3d

import (
	"testing"
)

// ── k3sVersionToImage ─────────────────────────────────────────────────────────

func TestK3sVersionToImage(t *testing.T) {
	tests := []struct {
		name       string
		k3sVersion string
		want       string
	}{
		{
			name:       "converts plus to dash",
			k3sVersion: "v1.28.10+k3s1",
			want:       "rancher/k3s:v1.28.10-k3s1",
		},
		{
			name:       "handles version without v prefix",
			k3sVersion: "1.28.10+k3s2",
			want:       "rancher/k3s:1.28.10-k3s2",
		},
		{
			name:       "handles multiple plus signs",
			k3sVersion: "v1.29.0+k3s1+build.20240101",
			want:       "rancher/k3s:v1.29.0-k3s1-build.20240101",
		},
		{
			name:       "handles version without plus",
			k3sVersion: "v1.28.10",
			want:       "rancher/k3s:v1.28.10",
		},
		{
			name:       "handles empty string",
			k3sVersion: "",
			want:       "rancher/k3s:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := k3sVersionToImage(tt.k3sVersion)
			if got != tt.want {
				t.Errorf("k3sVersionToImage(%q) = %q, want %q", tt.k3sVersion, got, tt.want)
			}
		})
	}
}

// ── firstLine ─────────────────────────────────────────────────────────────────

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "multiple lines",
			input: "first line\nsecond line\nthird line",
			want:  "first line",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "line with leading/trailing whitespace",
			input: "  test  \nother",
			want:  "test",
		},
		{
			name:  "newline only",
			input: "\n",
			want:  "",
		},
		{
			name:  "windows newlines",
			input: "first\r\nsecond",
			want:  "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstLine(tt.input)
			if got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
