package deploy

import "fmt"

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

func printBanner() {
	fmt.Printf("%s%s", colorCyan, colorBold)
	fmt.Println("  ██████╗  █████╗ ███╗   ██╗ ██████╗██╗  ██╗███████╗██████╗ ")
	fmt.Println("  ██╔══██╗██╔══██╗████╗  ██║██╔════╝██║  ██║██╔════╝██╔══██╗")
	fmt.Println("  ██████╔╝███████║██╔██╗ ██║██║     ███████║█████╗  ██████╔╝")
	fmt.Println("  ██╔══██╗██╔══██║██║╚██╗██║██║     ██╔══██║██╔══╝  ██╔══██╗")
	fmt.Println("  ██║  ██║██║  ██║██║ ╚████║╚██████╗██║  ██║███████╗██║  ██║")
	fmt.Println("  ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝")
	fmt.Printf("  %sDeployer%s\n", colorReset+colorBold, colorReset)
	fmt.Println()
}

func printStep(n int, msg string) {
	fmt.Printf("%s%s[%d] %s%s\n", colorBold, colorCyan, n, msg, colorReset)
}

func printInfo(format string, args ...any) {
	fmt.Printf("    "+format+"\n", args...)
}

func printSuccess(format string, args ...any) {
	fmt.Printf("%s%s✔ %s%s\n", colorBold, colorGreen, fmt.Sprintf(format, args...), colorReset)
}

func printWarning(format string, args ...any) {
	fmt.Printf("%s%s⚠ %s%s\n", colorBold, colorYellow, fmt.Sprintf(format, args...), colorReset)
}
