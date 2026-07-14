package k3d

import (
	"testing"
)

func TestK3sVersionToImage(t *testing.T) {
	tests := []struct {
		name       string
		k3sVersion string
		want       string
	}{
		{"converts plus to dash", "v1.28.10+k3s1", "rancher/k3s:v1.28.10-k3s1"},
		{"handles version without v prefix", "1.28.10+k3s2", "rancher/k3s:1.28.10-k3s2"},
		{"handles multiple plus signs", "v1.29.0+k3s1+build.20240101", "rancher/k3s:v1.29.0-k3s1-build.20240101"},
		{"handles version without plus", "v1.28.10", "rancher/k3s:v1.28.10"},
		{"handles empty string", "", "rancher/k3s:"},
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

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single line", "hello world", "hello world"},
		{"multiple lines", "first line\nsecond line\nthird line", "first line"},
		{"empty string", "", ""},
		{"line with leading/trailing whitespace", "  test  \nother", "test"},
		{"newline only", "\n", ""},
		{"windows newlines", "first\r\nsecond", "first"},
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
