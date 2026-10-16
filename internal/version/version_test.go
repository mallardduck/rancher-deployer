package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	// Version is set via ldflags, so default is "dev"
	got := Short()
	if got == "" {
		t.Error("Short() returned empty string")
	}
}

func TestInfo(t *testing.T) {
	got := Info("rancher-deployer")
	if got == "" {
		t.Error("Info() returned empty string")
	}

	// Should contain the binary name
	if !strings.Contains(got, "rancher-deployer") {
		t.Errorf("Info() should contain 'rancher-deployer', got: %s", got)
	}

	// Should contain go version
	if !strings.Contains(got, runtime.Version()) {
		t.Errorf("Info() should contain go version %s, got: %s", runtime.Version(), got)
	}

	// Should contain commit (even if "unknown")
	if !strings.Contains(got, "commit:") {
		t.Errorf("Info() should contain 'commit:', got: %s", got)
	}

	// Should contain build date (even if "unknown")
	if !strings.Contains(got, "built:") {
		t.Errorf("Info() should contain 'built:', got: %s", got)
	}
}

func TestInfoUsesBinaryName(t *testing.T) {
	for _, name := range []string{"rancher-deployer", "rancher-demo"} {
		first := strings.SplitN(Info(name), "\n", 2)[0]
		if want := name + " " + Version; first != want {
			t.Errorf("Info(%q) first line = %q, want %q", name, first, want)
		}
	}
	if strings.Contains(Info("rancher-demo"), "rancher-deployer") {
		t.Error("Info(\"rancher-demo\") should not mention rancher-deployer")
	}
}

func TestInfoFormat(t *testing.T) {
	// Verify the output is multiline
	info := Info("rancher-deployer")
	lines := strings.Split(info, "\n")
	if len(lines) < 4 {
		t.Errorf("Info() should have at least 4 lines, got %d", len(lines))
	}
}
