package kubectl

import "testing"

func TestParseServerVersion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"k3s", `{"serverVersion":{"gitVersion":"v1.28.10+k3s1"}}`, "v1.28.10+k3s1", false},
		{"plain", `{"serverVersion":{"gitVersion":"v1.30.2"}}`, "v1.30.2", false},
		{"no server", `{"clientVersion":{"gitVersion":"v1.30.2"}}`, "", true},
		{"empty version", `{"serverVersion":{"gitVersion":""}}`, "", true},
		{"not json", `connection refused`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServerVersion(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.28.10+k3s1":  "v1.28.10",
		"v1.29.4+rke2r1": "v1.29.4",
		"v1.30.2":        "v1.30.2",
	} {
		if got := BaseVersion(in); got != want {
			t.Errorf("BaseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithNamespace(t *testing.T) {
	if got := withNamespace("", "get", "x"); len(got) != 2 {
		t.Errorf("empty namespace should add no flags, got %v", got)
	}
	got := withNamespace("ns", "get", "x")
	if len(got) != 4 || got[2] != "-n" || got[3] != "ns" {
		t.Errorf("got %v", got)
	}
}
