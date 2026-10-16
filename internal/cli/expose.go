package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/expose"
	"github.com/mallardduck/rancher-deployer/internal/kubectl"
)

// ExposePlan is a resolved --expose mode plus everything deploy and upgrade
// need from it, so both commands drive the mode the same way.
type ExposePlan struct {
	Mode     expose.Mode
	Exposure expose.Exposure
	Steps    []expose.Step
	config   []string
}

// NewExposePlan resolves a mode against the user's hostname inputs.
func NewExposePlan(m expose.Mode, hostname string, helmSet []string, env expose.Env) (*ExposePlan, error) {
	exposure, err := expose.Resolve(m, hostname, helmSet)
	if err != nil {
		return nil, err
	}
	return &ExposePlan{
		Mode:     m,
		Exposure: exposure,
		Steps:    m.Steps(exposure, env),
		config:   m.ClusterConfig(env),
	}, nil
}

// PrintDryRun shows what the mode would ask for and apply.
func (p *ExposePlan) PrintDryRun() {
	if len(p.Steps) > 0 {
		fmt.Printf("\nSetup steps (%s):\n", p.Mode.Name())
		for i, s := range p.Steps {
			fmt.Printf("  %d. %s\n", i+1, s.Title)
			for _, l := range s.Lines {
				fmt.Printf("       %s\n", l)
			}
		}
	}
	for _, manifest := range p.config {
		fmt.Printf("\nCluster config applied (%s):\n%s", p.Mode.Name(), indentLines(manifest, "  "))
	}
}

// ApplyConfig applies the mode's cluster-level manifests.
func (p *ExposePlan) ApplyConfig() error {
	for _, manifest := range p.config {
		PrintInfo("Applying cluster config for --expose %s", p.Mode.Name())
		if err := kubectl.Apply(manifest); err != nil {
			return err
		}
	}
	return nil
}

// Verify warns, rather than fails, when Rancher is not reachable the way the
// mode promised.
func (p *ExposePlan) Verify(ctx context.Context) {
	if err := p.Mode.Verify(ctx, p.Exposure); err != nil {
		PrintWarning("Rancher is running but not reachable via --expose %s: %v", p.Mode.Name(), err)
	}
}

// RunExposeSteps walks the user through a mode's setup steps. Each step is
// shown, acknowledged, then verified with its Check. A failed Check can be
// retried or aborts the deploy. With yes set there is nobody to wait for, so
// steps are shown and a failed Check is an error.
func RunExposeSteps(ctx context.Context, steps []expose.Step, yes bool, in io.Reader) error {
	reader := bufio.NewReader(in)
	for i, s := range steps {
		fmt.Printf("\n%s── Setup %d/%d: %s%s\n", ColorCyan, i+1, len(steps), s.Title, ColorReset)
		for _, l := range s.Lines {
			fmt.Printf("   %s\n", l)
		}
		if !yes {
			fmt.Printf("\n%sPress Enter when done (Ctrl-C to abort): %s", ColorYellow, ColorReset)
			if _, err := reader.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
		}
		if s.Check == nil {
			continue
		}
		for {
			err := s.Check(ctx)
			if err == nil {
				PrintInfo("✓ %s", s.Title)
				break
			}
			PrintWarning("%s: %v", s.Title, err)
			if yes {
				return fmt.Errorf("setup step %q failed: %w", s.Title, err)
			}
			fmt.Printf("%sCheck again? [Y/n]: %s", ColorYellow, ColorReset)
			ans, _ := reader.ReadString('\n')
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "n") {
				return fmt.Errorf("setup step %q not completed", s.Title)
			}
		}
	}
	return nil
}

// indentLines prefixes every non-empty line of s with prefix.
func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
