package deploy

import (
	"reflect"
	"testing"

	"github.com/mallardduck/binup"
)

func TestConfirmPrompts(t *testing.T) {
	stable := &binup.Release{Version: "v1.0.0"}
	pre := &binup.Release{Version: "v1.1.0-rc.1"}

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
	tests := []struct {
		version string
		want    bool
	}{
		{"v1.0.0", false},
		{"1.0.0", false},
		{"v1.1.0-rc.1", true},
		{"1.1.0-beta", true},
		{"v1.0.0+build-5", false},
		{"v1.1.0-rc.1+abc", true},
		{"dev", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := isPrerelease(&binup.Release{Version: tt.version}); got != tt.want {
				t.Errorf("isPrerelease(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
