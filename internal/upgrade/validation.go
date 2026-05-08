// Package upgrade provides Rancher upgrade validation logic.
package upgrade

import (
	"fmt"
	"strings"
)

// ValidatePath checks if upgrading from current to target version is allowed.
// It enforces Rancher's upgrade rules:
//   - No downgrades
//   - No skipping minor versions (must upgrade one minor at a time)
//   - Patch upgrades within the same minor are always valid
//   - No cross-major upgrades
func ValidatePath(current, target string) error {
	cv := ParseMinorParts(current)
	tv := ParseMinorParts(target)

	if cv[0] != tv[0] {
		return fmt.Errorf(
			"cross-major upgrades are not supported (v%s → v%s)",
			current, target,
		)
	}

	minorDelta := tv[1] - cv[1]
	switch {
	case minorDelta < 0:
		return fmt.Errorf(
			"downgrade not supported: v%s is older than installed v%s",
			target, current,
		)
	case minorDelta > 1:
		return fmt.Errorf(
			"cannot skip minor versions: v%s → v%s skips %d minor release(s)\n"+
				"  Upgrade to v%d.%d.x first",
			current, target, minorDelta-1, cv[0], cv[1]+1,
		)
	}

	// Same minor: target patch must be >= current patch
	if minorDelta == 0 {
		cp := ParsePatchPart(current)
		tp := ParsePatchPart(target)
		if tp < cp {
			return fmt.Errorf(
				"downgrade not supported: v%s is older than installed v%s",
				target, current,
			)
		}
	}

	return nil
}

// ParseMinorParts returns [major, minor] as ints from a version string.
// Examples:
//   - "2.8.5" → [2, 8]
//   - "v2.9.0" → [2, 9]
func ParseMinorParts(v string) [2]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [2]int
	for i := 0; i < 2 && i < len(parts); i++ {
		_, _ = fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

// ParsePatchPart returns the patch integer from a version string.
// Examples:
//   - "2.8.5" → 5
//   - "v2.8.6" → 6
//   - "2.8" → 0 (no patch part)
func ParsePatchPart(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return 0
	}
	var patch int
	_, _ = fmt.Sscanf(parts[2], "%d", &patch)
	return patch
}
