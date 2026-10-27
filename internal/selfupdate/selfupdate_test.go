package selfupdate

import (
	"context"
	"os"
	"testing"

	"github.com/mallardduck/ghreleases/sigstore"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The fixtures are the checksum manifest and Sigstore bundle published with
// the real v0.6.0-rc.1 release, so this proves the pinned identity matches
// what release.yml actually signs.
func TestNewVerifier_AcceptsGenuineRelease(t *testing.T) {
	v, err := newVerifier(sigstore.WithTrustedRoot(fixture(t, "trusted_root.json")))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(context.Background(), fixture(t, "checksums.txt"), fixture(t, "checksums.txt.sigstore.json")); err != nil {
		t.Fatalf("genuine release rejected: %v", err)
	}
}

func TestNewVerifier_RejectsTamperedManifest(t *testing.T) {
	v, err := newVerifier(sigstore.WithTrustedRoot(fixture(t, "trusted_root.json")))
	if err != nil {
		t.Fatal(err)
	}
	tampered := append(fixture(t, "checksums.txt"), []byte("0000000000000000000000000000000000000000000000000000000000000000  evil.tar.gz\n")...)
	if err := v.Verify(context.Background(), tampered, fixture(t, "checksums.txt.sigstore.json")); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}
