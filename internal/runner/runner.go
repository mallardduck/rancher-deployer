// Package runner provides a thin wrapper around os/exec that prints commands
// before running them and captures output consistently.
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
)

// Run executes a command, streaming stdout/stderr to the terminal.
// The command is printed before execution.
func Run(name string, args ...string) error {
	printCmd(name, args)
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// RunSudo runs a command via sudo, streaming output.
func RunSudo(name string, args ...string) error {
	allArgs := append([]string{name}, args...)
	return Run("sudo", allArgs...)
}

// Output runs a command and returns its combined output as a string.
// Stderr is not shown unless the command fails.
func Output(name string, args ...string) (string, error) {
	printCmd(name, args)
	out, err := exec.Command(name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("command failed: %s %s\n%s", name, strings.Join(args, " "), s)
	}
	return s, nil
}

// Exists returns true if the binary is available in PATH.
func Exists(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// MustExist fatally exits if the binary is not found.
func MustExist(binary string) error {
	if !Exists(binary) {
		return fmt.Errorf("required binary %q not found in PATH — please install it first", binary)
	}
	return nil
}

func printCmd(name string, args []string) {
	fmt.Printf("  %s$%s %s %s\n", colorCyan, colorReset, name, strings.Join(args, " "))
}

func Helm(args ...string) error {
	return Run("helm", args...)
}

func K3d(args ...string) error {
	return Run("k3d", args...)
}

func Kubectl(args ...string) error {
	return Run("kubectl", args...)
}
