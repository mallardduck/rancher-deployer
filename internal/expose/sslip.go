package expose

import (
	"fmt"
	"net"
	"os"
)

// SSLIP is the default mode: Rancher is reached directly at the node's IP via
// an sslip.io hostname.
const SSLIP = "sslip"

type sslipMode struct{}

func (sslipMode) Name() string { return SSLIP }

// Resolve keeps a user-supplied hostname, otherwise builds
// rancher.<outbound-ip>.sslip.io. When the IP cannot be detected it warns and
// falls back to 127.0.0.1 rather than failing.
func (sslipMode) Resolve(userHostname string) (Exposure, error) {
	if userHostname != "" {
		return Exposure{Hostname: userHostname}, nil
	}
	ip, err := outboundIP()
	if err != nil {
		fmt.Printf("    Warning: could not detect node IP (%v) — using 127.0.0.1\n", err)
		fmt.Printf("    Set --hostname to override.\n")
		ip = "127.0.0.1"
	}
	return Exposure{Hostname: fmt.Sprintf("rancher.%s.sslip.io", ip)}, nil
}

// outboundIP returns the preferred outbound IP of the local machine.
// In containers, always returns 127.0.0.1 since the container's bridge IP
// (172.17.0.x) is not accessible from the host.
func outboundIP() (string, error) {
	// Detect if running in a container
	if inContainer() {
		return "127.0.0.1", nil
	}

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String(), nil
}

// inContainer detects if we're running inside a container.
// Checks for /.dockerenv file or CONTAINER environment variable.
func inContainer() bool {
	// Docker creates /.dockerenv in containers
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Also check for explicit CONTAINER env var
	if os.Getenv("CONTAINER") != "" {
		return true
	}
	return false
}
