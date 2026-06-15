// Package deployment provides deployment workflow utilities.
package deployment

import (
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/detect"
)

// ResolveMode returns the effective install mode ("k3s", "k3d", or "existing").
// If flag is empty, it auto-detects the appropriate mode based on OS and available tools.
// Valid flag values: "", "k3s", "k3d", "existing" (case-insensitive).
//
// "existing" mode assumes a Kubernetes cluster already exists and is accessible via kubectl.
func ResolveMode(flag string, autoMessage bool) (string, string, error) {
	switch strings.ToLower(flag) {
	case "k3s":
		return "k3s", "", nil
	case "k3d":
		return "k3d", "", nil
	case "existing":
		return "existing", "", nil
	case "":
		mode, reason := detect.InstallMode()
		return mode, reason, nil
	default:
		return "", "", fmt.Errorf("invalid mode %q: must be 'k3s', 'k3d', or 'existing'", flag)
	}
}
