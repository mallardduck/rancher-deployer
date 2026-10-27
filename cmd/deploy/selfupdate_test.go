package deploy

import (
	"reflect"
	"testing"

	"github.com/mallardduck/binup"
)

func TestConfirmPrompts(t *testing.T) {
	stable := &binup.Release{Version: "v1.0.0"}
	pre := &binup.Release{Version: "v1.1.0-rc.1", Prerelease: true}
	// GitHub's flag is authoritative: a plain tag can still be a prerelease.
	flagged := &binup.Release{Version: "v1.1.0", Prerelease: true}

	tests := []struct {
		name string
		f    selfUpdateFlags
		rel  *binup.Release
		want []string
	}{
		{"stable asks once", selfUpdateFlags{}, stable, []string{"Update to v1.0.0?"}},
		{"stable with --yes asks nothing", selfUpdateFlags{yes: true}, stable, nil},
		{
			"prerelease asks twice",
			selfUpdateFlags{pre: true}, pre,
			[]string{"Update to v1.1.0-rc.1?", "v1.1.0-rc.1 is a prerelease and may be unstable. Install it anyway?"},
		},
		{"--pre --yes is the explicit script opt-in", selfUpdateFlags{pre: true, yes: true}, pre, nil},
		{
			"flagged prerelease with a plain tag still asks twice",
			selfUpdateFlags{pre: true}, flagged,
			[]string{"Update to v1.1.0?", "v1.1.0 is a prerelease and may be unstable. Install it anyway?"},
		},
		{"flagged prerelease with --yes alone still asks the prerelease question", selfUpdateFlags{yes: true}, flagged, []string{"v1.1.0 is a prerelease and may be unstable. Install it anyway?"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := confirmPrompts(&tt.f, tt.rel)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("confirmPrompts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsPrerelease(t *testing.T) {
	for v, want := range map[string]bool{
		"v1.1.0-rc.2":      true,
		"1.1.0-rc.2":       true,
		"v1.1.0":           false,
		"v1.1.0+build-5":   false,
		"v1.1.0-rc.2+meta": true,
		"dev":              false,
		"":                 false,
	} {
		if got := isPrerelease(v); got != want {
			t.Errorf("isPrerelease(%q) = %v, want %v", v, got, want)
		}
	}
}
