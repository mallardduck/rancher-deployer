package existing

import (
	"encoding/json"
	"strings"
)

// kubectlVersionOutput represents the JSON output from kubectl version --output=json
type kubectlVersionOutput struct {
	ServerVersion *versionInfo `json:"serverVersion"`
}

// versionInfo contains the git version string
type versionInfo struct {
	GitVersion string `json:"gitVersion"`
}

// extractVersion parses the Kubernetes version from kubectl version JSON output.
// Handles both standard k8s versions (v1.28.10) and k3s versions (v1.28.10+k3s1).
// Strips the +k3s suffix to return the base k8s version.
func extractVersion(output string) string {
	var versionOutput kubectlVersionOutput
	if err := json.Unmarshal([]byte(output), &versionOutput); err != nil {
		return ""
	}

	if versionOutput.ServerVersion == nil {
		return ""
	}

	fullVersion := versionOutput.ServerVersion.GitVersion
	if fullVersion == "" {
		return ""
	}

	// Strip +k3s suffix if present to get base k8s version
	if plusIdx := strings.Index(fullVersion, "+"); plusIdx != -1 {
		return fullVersion[:plusIdx] // e.g., v1.28.10
	}

	return fullVersion
}

// extractMinor extracts the major.minor portion of a version string.
// e.g., "1.28.10" -> "1.28", "1.29.0" -> "1.29"
func extractMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
