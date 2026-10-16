// Package kubectl wraps the kubectl invocations shared by the cluster
// providers, helm backends and Rancher readiness checks. It is a thin layer
// over runner: it knows how to ask kubectl questions, not what Rancher is.
package kubectl

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// ServerVersion returns the raw server version reported by the cluster,
// including any distro suffix (e.g. "v1.28.10+k3s1", "v1.29.4+rke2r1").
func ServerVersion() (string, error) {
	out, err := runner.Output("kubectl", "version", "--output=json")
	if err != nil {
		return "", fmt.Errorf("could not query cluster version: %w", err)
	}
	return parseServerVersion(out)
}

func parseServerVersion(out string) (string, error) {
	var v struct {
		ServerVersion *struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.ServerVersion == nil {
		return "", fmt.Errorf("could not parse kubectl version output")
	}
	if v.ServerVersion.GitVersion == "" {
		return "", fmt.Errorf("kubectl version returned an empty server version")
	}
	return v.ServerVersion.GitVersion, nil
}

// BaseVersion strips any distro suffix from a server version
// (e.g. "v1.28.10+k3s1" → "v1.28.10").
func BaseVersion(version string) string {
	if i := strings.Index(version, "+"); i != -1 {
		return version[:i]
	}
	return version
}

// Apply writes manifest to a temporary file and applies it.
func Apply(manifest string) error {
	f, err := os.CreateTemp("", "manifest-*.yaml")
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

// ApplyURL applies the manifest served at url.
func ApplyURL(url string) error {
	return runner.Kubectl("apply", "-f", url)
}

// CreateNamespace creates the named namespace.
func CreateNamespace(name string) error {
	return runner.Kubectl("create", "namespace", name)
}

// Exists reports whether the resource exists. Pass "" for cluster-scoped
// resources or to use the default namespace.
func Exists(kind, name, namespace string) (bool, error) {
	out, err := runner.Output("kubectl", withNamespace(namespace, "get", kind, name, "--ignore-not-found", "-o", "name")...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// JSONPath returns the value of a jsonpath expression (without the braces)
// evaluated against the resource.
func JSONPath(kind, name, namespace, expr string) (string, error) {
	return runner.Output("kubectl", withNamespace(namespace, "get", kind, name, "-o", "jsonpath={"+expr+"}")...)
}

// PodsTable returns the `get pods --no-headers` table for pods matching selector.
func PodsTable(namespace, selector string) (string, error) {
	return runner.Output("kubectl", "get", "pods", "-n", namespace, "-l", selector, "--no-headers")
}

// RolloutStatus waits for a deployment rollout to finish.
func RolloutStatus(namespace, deployment string, timeout time.Duration) error {
	return runner.Kubectl("rollout", "status", "deployment/"+deployment,
		"-n", namespace, fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())))
}

// WaitFor waits for condition (e.g. "condition=complete") on resource
// (e.g. "job/foo"). Pass "" as namespace for cluster-scoped resources.
func WaitFor(condition, resource, namespace string, timeout time.Duration) error {
	return runner.Kubectl(withNamespace(namespace, "wait", "--for="+condition, resource,
		fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())))...)
}

func withNamespace(namespace string, args ...string) []string {
	if namespace == "" {
		return args
	}
	return append(args, "-n", namespace)
}
