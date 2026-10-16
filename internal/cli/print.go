// Package cli holds the CLI plumbing shared by the rancher-deployer and rancher-demo binaries.
package cli

import (
	"fmt"
	"os"
	"strings"
)

const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorCyan   = "\033[36m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorRed    = "\033[31m"
)

// PrintBanner prints the ASCII banner followed by title.
func PrintBanner(title string) {
	fmt.Printf("%s%s", ColorCyan, ColorBold)
	fmt.Println("  ██████╗  █████╗ ███╗   ██╗ ██████╗██╗  ██╗███████╗██████╗ ")
	fmt.Println("  ██╔══██╗██╔══██╗████╗  ██║██╔════╝██║  ██║██╔════╝██╔══██╗")
	fmt.Println("  ██████╔╝███████║██╔██╗ ██║██║     ███████║█████╗  ██████╔╝")
	fmt.Println("  ██╔══██╗██╔══██║██║╚██╗██║██║     ██╔══██║██╔══╝  ██╔══██╗")
	fmt.Println("  ██║  ██║██║  ██║██║ ╚████║╚██████╗██║  ██║███████╗██║  ██║")
	fmt.Println("  ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝")
	fmt.Printf("  %s%s%s\n", ColorReset+ColorBold, title, ColorReset)
	fmt.Println()
}

// PrintStep prints a step line.
func PrintStep(n int, msg string) {
	fmt.Printf("%s%s[%d] %s%s\n", ColorBold, ColorCyan, n, msg, ColorReset)
}

// PrintInfo prints a info line.
func PrintInfo(format string, args ...any) {
	fmt.Printf("    "+format+"\n", args...)
}

// PrintSuccess prints a success line.
func PrintSuccess(format string, args ...any) {
	fmt.Printf("%s%s✔ %s%s\n", ColorBold, ColorGreen, fmt.Sprintf(format, args...), ColorReset)
}

// PrintWarning prints a warning line.
func PrintWarning(format string, args ...any) {
	fmt.Printf("%s%s⚠ %s%s\n", ColorBold, ColorYellow, fmt.Sprintf(format, args...), ColorReset)
}

// PromptConfirm asks for a yes/no answer on stdin.
func PromptConfirm(prompt string) bool {
	fmt.Printf("\n%s%s [y/N]: %s", ColorYellow, prompt, ColorReset)
	var answer string
	_, _ = fmt.Fscanln(os.Stdin, &answer)
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}
