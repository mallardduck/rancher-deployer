package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// ClusterAccessChecker tests kubectl cluster connectivity.
type ClusterAccessChecker struct{}

func (c *ClusterAccessChecker) Name() string {
	return "Kubernetes cluster access"
}

func (c *ClusterAccessChecker) Category() string {
	return "state"
}

func (c *ClusterAccessChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	if !runner.Exists("kubectl") {
		// kubectl not available, skip this check
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "kubectl not available",
		}
	}

	cmd := exec.CommandContext(ctx, "kubectl", "cluster-info", "--request-timeout=5s")
	out, err := cmd.CombinedOutput()

	if err == nil {
		// Cluster is accessible
		outStr := strings.TrimSpace(string(out))
		// Extract first line (usually the control plane info)
		lines := strings.Split(outStr, "\n")
		msg := "cluster is accessible"
		if len(lines) > 0 && strings.Contains(lines[0], "running at") {
			msg = strings.TrimSpace(lines[0])
		}

		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  msg,
		}
	}

	// Check if it's a "no cluster" error vs an actual error
	errMsg := string(out)
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "was refused") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "Unable to connect") {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     "no cluster accessible",
			Remediation: "This is normal if you haven't deployed yet",
		}
	}

	// Some other error
	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      StatusWarn,
		Message:     fmt.Sprintf("cluster check failed: %v", err),
		Remediation: "Verify your kubeconfig is valid",
	}
}

// K3sServiceChecker checks k3s systemd service status.
type K3sServiceChecker struct{}

func (c *K3sServiceChecker) Name() string {
	return "k3s service status"
}

func (c *K3sServiceChecker) Category() string {
	return "state"
}

func (c *K3sServiceChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	if !runner.Exists("systemctl") {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "systemctl not available",
		}
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "k3s")
	out, err := cmd.CombinedOutput()
	status := strings.TrimSpace(string(out))

	if err == nil && status == "active" {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "k3s service is running",
		}
	}

	if status == "inactive" || status == "failed" {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     fmt.Sprintf("k3s service is %s", status),
			Remediation: "Start k3s: sudo systemctl start k3s",
		}
	}

	// Service not found (normal for fresh install)
	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  "k3s service not installed (expected for new deployment)",
	}
}

// K3dClusterChecker lists existing k3d clusters.
type K3dClusterChecker struct{}

func (c *K3dClusterChecker) Name() string {
	return "k3d clusters"
}

func (c *K3dClusterChecker) Category() string {
	return "state"
}

func (c *K3dClusterChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	if !runner.Exists("k3d") {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "k3d not installed yet",
		}
	}

	cmd := exec.CommandContext(ctx, "k3d", "cluster", "list", "-o", "json")
	out, err := cmd.CombinedOutput()

	if err != nil {
		// k3d list failed
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "could not list k3d clusters",
		}
	}

	// Parse JSON output
	var clusters []struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(out, &clusters); err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "could not parse k3d cluster list",
		}
	}

	if len(clusters) == 0 {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "no existing clusters found",
		}
	}

	// List cluster names
	names := make([]string, len(clusters))
	for i, c := range clusters {
		names[i] = c.Name
	}

	msg := fmt.Sprintf("found %d cluster(s): %s", len(clusters), strings.Join(names, ", "))

	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  msg,
	}
}

// RancherInstallationChecker checks for existing Rancher deployment.
type RancherInstallationChecker struct{}

func (c *RancherInstallationChecker) Name() string {
	return "Rancher installation"
}

func (c *RancherInstallationChecker) Category() string {
	return "state"
}

func (c *RancherInstallationChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	if !runner.Exists("helm") {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "helm not available",
		}
	}

	cmd := exec.CommandContext(ctx, "helm", "list", "-n", "cattle-system", "-o", "json")
	out, err := cmd.CombinedOutput()

	if err != nil {
		// helm list failed (namespace might not exist)
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "Rancher not installed",
		}
	}

	// Parse JSON output
	var releases []struct {
		Name       string `json:"name"`
		AppVersion string `json:"app_version"`
		Status     string `json:"status"`
	}

	if err := json.Unmarshal(out, &releases); err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "could not parse helm list output",
		}
	}

	// Look for rancher release
	for _, r := range releases {
		if r.Name == "rancher" {
			version := strings.TrimPrefix(r.AppVersion, "v")
			msg := fmt.Sprintf("installed (v%s, status: %s)", version, r.Status)

			status := StatusPass
			if r.Status != "deployed" {
				status = StatusWarn
			}

			return CheckResult{
				Name:     c.Name(),
				Category: c.Category(),
				Status:   status,
				Message:  msg,
			}
		}
	}

	// Rancher not found
	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  "Rancher not installed",
	}
}
