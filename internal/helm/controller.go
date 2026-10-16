package helm

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mallardduck/rancher-deployer/internal/kubectl"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
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
	out, err := kubectl.JSONPath("helmchart", release, "kube-system", ".spec.valuesContent")
	if err != nil {
		return "", fmt.Errorf("could not get HelmChart CR %q: %w", release, err)
	}
	return out, nil
}

func (c *Controller) IsInstalled(release, _ string) (bool, error) {
	return kubectl.Exists("helmchart", release, "kube-system")
}

func (c *Controller) InstalledVersion(_ string) (string, error) {
	out, err := kubectl.JSONPath("helmchart", "rancher", "kube-system", ".spec.version")
	if err != nil {
		return "", fmt.Errorf("could not get installed version from HelmChart CR: %w", err)
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "", fmt.Errorf("rancher HelmChart CR not found in kube-system")
	}
	return strings.TrimPrefix(v, "v"), nil
}

const (
	certManagerRepo      = "https://charts.jetstack.io"
	certManagerChartName = "cert-manager"
	certManagerNamespace = "cert-manager"
)

// InstallCertManager creates a HelmChart CR for cert-manager with installCRDs=true,
// waits for the helm-controller Job to complete, then waits for all cert-manager
// deployments to be ready (required before Rancher's webhook-dependent install).
func (c *Controller) InstallCertManager(version string) error {
	already, err := c.IsInstalled(certManagerChartName, certManagerNamespace)
	if err != nil {
		return err
	}
	if already {
		return fmt.Errorf(
			"HelmChart CR %q already exists in kube-system\n"+
				"  Re-runs are not supported. Remove it before retrying:\n"+
				"    kubectl delete helmchart %s -n kube-system",
			certManagerChartName, certManagerChartName,
		)
	}

	chart := rancher.Chart{
		ChartName: certManagerChartName,
		RepoURL:   certManagerRepo,
		Version:   version,
	}
	if err := c.applyChartCR(certManagerNamespace, chart, rancher.HelmValues{
		SetFlags: []string{"installCRDs=true"},
	}); err != nil {
		return err
	}

	if err := waitForHelmJob(certManagerChartName, 5*time.Minute); err != nil {
		return fmt.Errorf("cert-manager helm-controller job failed: %w", err)
	}

	for _, deploy := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
		if err := kubectl.RolloutStatus(certManagerNamespace, deploy, 120*time.Second); err != nil {
			return fmt.Errorf("cert-manager deployment %q did not become ready: %w", deploy, err)
		}
	}
	return nil
}

// WaitReady waits for the helm-controller Job to finish installing Rancher, then
// waits for the Rancher deployment itself to become available.
func (c *Controller) WaitReady(namespace string) error {
	if err := waitForHelmJob("rancher", 10*time.Minute); err != nil {
		return fmt.Errorf("rancher helm-controller job failed: %w", err)
	}
	return rancher.WaitReady(namespace)
}

// waitForHelmJob polls until the helm-controller Job for releaseName appears in
// kube-system, then waits for it to reach the Complete condition.
func waitForHelmJob(releaseName string, timeout time.Duration) error {
	jobName := "helm-install-" + releaseName
	fmt.Printf("  Waiting for helm-controller job/%s ...\n", jobName)
	deadline := time.Now().Add(timeout)

	// Poll until the Job is created (helm-controller may take a few seconds)
	for time.Now().Before(deadline) {
		if exists, _ := kubectl.Exists("job", jobName, "kube-system"); exists {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("timed out waiting for job/%s to be created", jobName)
	}

	remaining := time.Until(deadline)
	if err := kubectl.WaitFor("condition=complete", "job/"+jobName, "kube-system", remaining); err != nil {
		// Check whether the job explicitly failed rather than just timing out
		out, _ := kubectl.JSONPath("job", jobName, "kube-system", ".status.failed")
		if f := strings.TrimSpace(out); f != "" && f != "0" {
			return fmt.Errorf("job/%s failed — inspect with: kubectl logs -n kube-system -l job-name=%s", jobName, jobName)
		}
		return fmt.Errorf("job/%s did not complete: %w", jobName, err)
	}
	return nil
}

// ── HelmChart CR template ─────────────────────────────────────────────────────

type helmChartData struct {
	ChartName       string
	RepoURL         string
	Version         string
	TargetNamespace string
	FailurePolicy   string
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
  failurePolicy: {{ .FailurePolicy }}
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

	failurePolicy, err := rancher.ParseFailurePolicy(values.FailurePolicy)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := helmChartCRTmpl.Execute(&buf, helmChartData{
		ChartName:       chart.ChartName,
		RepoURL:         chart.RepoURL,
		Version:         chart.Version,
		TargetNamespace: namespace,
		FailurePolicy:   failurePolicy,
		ValuesContent:   indentBlock(strings.TrimRight(valuesContent, "\n"), "    "),
	}); err != nil {
		return fmt.Errorf("rendering HelmChart manifest: %w", err)
	}

	return kubectl.Apply(buf.String())
}

// buildValuesContent deep-merges a values file and --set flags into a single
// YAML string suitable for a HelmChart CR's spec.valuesContent field.
//
// The values file is the base layer; --set flags are applied on top with
// last-value-wins semantics for overlapping keys. Everything is emitted as a
// single YAML document so helm-controller can pass it as a --values file
// without YAML parse errors.
func buildValuesContent(values rancher.HelmValues) (string, error) {
	merged := make(map[string]any)

	if values.ValuesFile != "" {
		data, err := os.ReadFile(values.ValuesFile) //nolint:gosec // path validated upstream by BuildHelmValues
		if err != nil {
			return "", fmt.Errorf("reading values file %q: %w", values.ValuesFile, err)
		}
		if err := yaml.Unmarshal(data, &merged); err != nil {
			return "", fmt.Errorf("parsing values file %q: %w", values.ValuesFile, err)
		}
	}

	for _, flag := range values.SetFlags {
		k, v, ok := strings.Cut(flag, "=")
		if !ok {
			continue
		}
		setNestedValue(merged, strings.Split(k, "."), parseSetValue(v))
	}

	if len(merged) == 0 {
		return "", nil
	}

	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("serialising values: %w", err)
	}
	return string(out), nil
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
