// Package expose decides how a deployed Rancher will be reached from outside
// the cluster (--expose). It is a rancher-deployer concern: the Rancher chart
// only ever sees the plain data an Exposure turns into (a hostname and --set
// values), so nothing here is imported by the rancher or helm packages.
package expose

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Exposure is the outcome of a mode: the hostname for the chart, plus any
// chart values the mode requires.
type Exposure struct {
	Hostname string
	SetFlags []string // key=value entries
}

// MergeSets appends the exposure's chart values to sets, skipping any key the
// caller already set so explicit user values always win.
func (e Exposure) MergeSets(sets []string) []string {
	out := append([]string(nil), sets...)
	for _, s := range e.SetFlags {
		key, _, ok := strings.Cut(s, "=")
		if ok && !hasKey(out, key) {
			out = append(out, s)
		}
	}
	return out
}

func hasKey(sets []string, key string) bool {
	for _, s := range sets {
		if strings.HasPrefix(s, key+"=") {
			return true
		}
	}
	return false
}

// Env holds facts about the deploy that a mode may need to word its steps.
type Env struct {
	ClusterMode string // k3s, k3d or existing
	Namespace   string
}

// Step is one manual setup action a mode needs from the user before the
// deploy continues. The mode supplies data; cmd/deploy renders and prompts.
type Step struct {
	Title string
	Lines []string // instructions, with computed values already filled in
	// Check, if set, confirms the step took effect. A nil Check means the
	// step is only acknowledged by the user.
	Check func(context.Context) error
}

// Mode is one way of exposing Rancher.
type Mode interface {
	Name() string
	// Resolve receives the hostname the user supplied (via --hostname or
	// --set hostname=), which may be empty. A mode may accept it, derive one
	// when it is empty, or reject it.
	Resolve(userHostname string) (Exposure, error)
	// ClusterConfig returns YAML manifests to apply once the cluster exists
	// and before Rancher is installed. Modes that need nothing return none.
	ClusterConfig(Env) []string
	// Steps returns the ordered setup the user must complete before anything
	// is installed. A mode with nothing to ask returns none.
	Steps(Exposure, Env) []Step
	// Verify runs after Rancher is ready and confirms it is reachable the way
	// the mode promised. Modes with nothing to verify return nil.
	Verify(context.Context, Exposure) error
}

// New returns the Mode for an --expose value, defaulting "" to SSLIP.
func New(name string) (Mode, error) {
	switch strings.ToLower(name) {
	case "", SSLIP:
		return sslipMode{}, nil
	case Pangolin:
		return newPangolin(), nil
	case "ngrok":
		return nil, errors.New("--expose ngrok is not implemented yet — contributions welcome (see issue #12)")
	default:
		return nil, fmt.Errorf("unknown --expose mode %q: must be %s or %s", name, SSLIP, Pangolin)
	}
}

// Resolve applies a mode to the user's hostname inputs. A --hostname flag wins
// over a hostname= entry in setFlags.
func Resolve(m Mode, hostnameFlag string, setFlags []string) (Exposure, error) {
	e, err := m.Resolve(userHostname(hostnameFlag, setFlags))
	if err != nil {
		return Exposure{}, fmt.Errorf("--expose %s: %w", m.Name(), err)
	}
	return e, nil
}

func userHostname(flag string, setFlags []string) string {
	if flag != "" {
		return flag
	}
	for _, s := range setFlags {
		if h, ok := strings.CutPrefix(s, "hostname="); ok {
			return h
		}
	}
	return ""
}
