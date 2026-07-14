package helm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/mallardduck/rancher-deployer/internal/rancher"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

var _ Backend = (*Controller)(nil)

// Controller implements Backend using k3s/rke2's built-in helm-controller.
// Instead of running helm CLI commands, it creates and manages HelmChart CRDs
// in the kube-system namespace, which the in-cluster controller reconciles.
// This means the helm binary is not required on the host.
type Controller struct{}

// NewController returns a helm Backend that uses the cluster's helm-controller.
func NewController() *Controller { return &Controller{} }

// EnsureRepo is a no-op: the helm-controller takes the chart repo URL directly
// from the HelmChart CR's spec.repo field — no local repo registration needed.
func (c *Controller) EnsureRepo(_, _ string, _ bool) error { return nil }

func (c *Controller) Install(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	already, err := c.IsInstalled(chart.ChartName, namespace)
	if err != nil {
		return err
	}
	if already {
		return fmt.Errorf(
			"HelmChart CR %q already exists in kube-system\n"+
				"  Re-runs are not supported. Remove it before retrying:\n"+
				"    kubectl delete helmchart %s -n kube-system",
			chart.ChartName, chart.ChartName,
		)
	}
	return c.applyChartCR(namespace, chart, values)
}

// Upgrade patches the existing HelmChart CR to the new version.
// kubectl apply is idempotent so this also handles the install case cleanly.
func (c *Controller) Upgrade(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	return c.applyChartCR(namespace, chart, values)
}

func (c *Controller) GetValues(release, _ string) (string, error) {
	out, err := runner.Output("kubectl", "get", "helmchart", release,
		"-n", "kube-system",
		"-o", "jsonpath={.spec.valuesContent}",
	)
	if err != nil {
		return "", fmt.Errorf("could not get HelmChart CR %q: %w", release, err)
	}
	return out, nil
}

func (c *Controller) IsInstalled(release, _ string) (bool, error) {
	out, err := runner.Output("kubectl", "get", "helmchart", release,
		"-n", "kube-system",
		"--ignore-not-found",
		"-o", "name",
	)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (c *Controller) InstalledVersion(_ string) (string, error) {
	out, err := runner.Output("kubectl", "get", "helmchart", "rancher",
		"-n", "kube-system",
		"-o", "jsonpath={.spec.version}",
	)
	if err != nil {
		return "", fmt.Errorf("could not get installed version from HelmChart CR: %w", err)
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "", fmt.Errorf("rancher HelmChart CR not found in kube-system")
	}
	return strings.TrimPrefix(v, "v"), nil
}

// ── HelmChart CR template ─────────────────────────────────────────────────────

type helmChartData struct {
	ChartName       string
	RepoURL         string
	Version         string
	TargetNamespace string
	ValuesContent   string // pre-indented; empty string omits the field entirely
}

// helmChartCRTmpl is the manifest template for a HelmChart CR.
// ValuesContent must arrive pre-indented to 4 spaces; the template emits it
// inside a literal block scalar (|-) under spec.valuesContent.
var helmChartCRTmpl = template.Must(template.New("helmchart").Parse(`apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: {{ .ChartName }}
  namespace: kube-system
spec:
  chart: {{ .ChartName }}
  repo: {{ .RepoURL }}
  version: {{ .Version }}
  targetNamespace: {{ .TargetNamespace }}
  createNamespace: true
{{- if .ValuesContent }}
  valuesContent: |-
{{ .ValuesContent }}
{{- end }}
`))

// ── internal helpers ─────────────────────────────────────────────────────────

func (c *Controller) applyChartCR(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	valuesContent, err := buildValuesContent(values)
	if err != nil {
		return fmt.Errorf("building valuesContent: %w", err)
	}

	var buf bytes.Buffer
	if err := helmChartCRTmpl.Execute(&buf, helmChartData{
		ChartName:       chart.ChartName,
		RepoURL:         chart.RepoURL,
		Version:         chart.Version,
		TargetNamespace: namespace,
		ValuesContent:   indentBlock(strings.TrimRight(valuesContent, "\n"), "    "),
	}); err != nil {
		return fmt.Errorf("rendering HelmChart manifest: %w", err)
	}

	return applyManifest(buf.String())
}

// buildValuesContent merges a values file and --set flags into a single string
// suitable for a HelmChart CR's spec.valuesContent field.
//
// Values file content is emitted first (base layer). The --set flags are
// serialized to JSON and appended after — JSON is valid YAML and last-value-wins
// under most parsers, giving the flags correct override precedence over the file.
func buildValuesContent(values rancher.HelmValues) (string, error) {
	var parts []string

	if values.ValuesFile != "" {
		data, err := os.ReadFile(values.ValuesFile) //nolint:gosec // path validated upstream by BuildHelmValues
		if err != nil {
			return "", fmt.Errorf("reading values file %q: %w", values.ValuesFile, err)
		}
		if trimmed := strings.TrimRight(string(data), "\n"); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if len(values.SetFlags) > 0 {
		m := setFlagsToMap(values.SetFlags)
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return "", fmt.Errorf("serialising --set flags: %w", err)
		}
		parts = append(parts, string(b))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n") + "\n", nil
}

// setFlagsToMap converts helm --set flags (dot-notation key=value pairs) into a
// nested map with type-aware values (numbers and booleans stay their native type).
func setFlagsToMap(flags []string) map[string]any {
	m := make(map[string]any)
	for _, flag := range flags {
		k, v, ok := strings.Cut(flag, "=")
		if !ok {
			continue
		}
		setNestedValue(m, strings.Split(k, "."), parseSetValue(v))
	}
	return m
}

// setNestedValue sets a pre-parsed value into a nested map using a key path.
func setNestedValue(m map[string]any, keys []string, value any) {
	if len(keys) == 1 {
		m[keys[0]] = value
		return
	}
	sub, ok := m[keys[0]].(map[string]any)
	if !ok {
		sub = make(map[string]any)
		m[keys[0]] = sub
	}
	setNestedValue(sub, keys[1:], value)
}

// parseSetValue converts a raw --set value string to the most specific Go type.
// This ensures json.Marshal emits numbers and booleans without quotes.
func parseSetValue(s string) any {
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if s == "null" || s == "~" {
		return nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// indentBlock prepends prefix to every non-empty line of content.
func indentBlock(content, prefix string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// applyManifest writes the manifest to a temp file and applies it with kubectl.
func applyManifest(manifest string) error {
	f, err := os.CreateTemp("", "helmchart-*.yaml")
	if err != nil {
		return fmt.Errorf("could not create temp manifest: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }() //nolint:gosec // path from os.CreateTemp
	if _, err := f.WriteString(manifest); err != nil {
		return fmt.Errorf("could not write manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("could not close manifest file: %w", err)
	}
	return runner.Kubectl("apply", "-f", f.Name())
}
