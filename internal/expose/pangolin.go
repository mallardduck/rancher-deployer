package expose

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Pangolin is the mode for a Pangolin tunnel (and any similar reverse tunnel
// that terminates TLS at its edge): the user owns the public hostname, the
// tunnel forwards plain HTTP to the node, and Rancher sits behind it with
// TLS handled externally.
const Pangolin = "pangolin"

// pangolinTarget is where the tunnel must forward to. k3s and k3d both
// publish the ingress on host port 80, and tls=external makes that plain HTTP.
const pangolinTarget = "http://127.0.0.1:80"

// errAuthRequired means the tunnel answered but wants a login first, e.g. the
// Pangolin resource has SSO enabled. Rancher must be public: Rancher has its
// own login, and downstream cluster agents cannot complete a tunnel SSO flow.
var errAuthRequired = errors.New("the tunnel is asking for authentication (is Pangolin SSO enabled on the resource?) — " +
	"Rancher must be reachable without it: it has its own login, and downstream cluster agents cannot sign in through Pangolin")

type pangolinMode struct {
	lookupHost func(ctx context.Context, host string) ([]string, error)
	client     *http.Client
	pingURL    func(host string) string
	timeout    time.Duration
	interval   time.Duration
}

func newPangolin() pangolinMode {
	return pangolinMode{
		lookupHost: net.DefaultResolver.LookupHost,
		client:     &http.Client{Timeout: 10 * time.Second},
		pingURL:    func(host string) string { return "https://" + host + "/ping" },
		timeout:    90 * time.Second,
		interval:   3 * time.Second,
	}
}

func (pangolinMode) Name() string { return Pangolin }

// Resolve requires the public hostname — there is no way to discover it from
// the tunnel — and tells the chart that TLS is handled outside the cluster.
func (pangolinMode) Resolve(userHostname string) (Exposure, error) {
	switch {
	case userHostname == "":
		return Exposure{}, errors.New("--hostname <public fqdn> is required (the domain of your Pangolin resource)")
	case strings.Contains(userHostname, "://") || strings.Contains(userHostname, "/"):
		return Exposure{}, fmt.Errorf("--hostname %q must be a bare hostname, e.g. rancher.example.com", userHostname)
	}
	return Exposure{Hostname: userHostname, SetFlags: []string{"tls=external"}}, nil
}

// traefikTrustForwarded makes the k3s-bundled Traefik keep the tunnel's
// X-Forwarded-Proto header. By default Traefik replaces it with the scheme of
// its own entrypoint (http), which makes Rancher (tls=external) redirect to
// https forever. insecure trusts forwarded headers from any client, so it is
// only appropriate for the dev/demo setups this tool targets.
const traefikTrustForwarded = `apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: traefik
  namespace: kube-system
spec:
  valuesContent: |-
    ports:
      web:
        forwardedHeaders:
          insecure: true
`

// ClusterConfig configures the bundled Traefik on k3s and k3d. Other clusters
// have unknown ingress controllers, so the user is told what to set instead.
func (pangolinMode) ClusterConfig(env Env) []string {
	switch env.ClusterMode {
	case "k3s", "k3d":
		return []string{traefikTrustForwarded}
	default:
		return nil
	}
}

func (m pangolinMode) Steps(e Exposure, env Env) []Step {
	target := pangolinTarget
	var ingressNote []string
	if env.ClusterMode == "existing" {
		target = "http://<ingress address reachable from the site>:80 (plain HTTP port of your ingress controller)"
		ingressNote = []string{
			"Your ingress controller must pass the tunnel's X-Forwarded-Proto",
			"header through to Rancher. (Traefik: trust forwarded headers on the",
			"web entrypoint. Without it Rancher redirects to https in a loop.)",
		}
	}
	return []Step{
		{
			Title: "Start the tunnel",
			Lines: []string{
				"In Pangolin, create a Site and run the connect command it shows",
				"on this machine. Keep it running (use a second terminal if it",
				"runs in the foreground).",
			},
		},
		{
			Title: "Create the public resource",
			Lines: append([]string{
				"In Pangolin, add a public Resource:",
				"  Domain : " + e.Hostname,
				"  Site   : the site from the previous step",
				"  Target : " + target,
				"Rancher is not installed yet, so it will show a gateway error until it is.",
				"Leave Pangolin authentication (SSO) OFF for this resource: Rancher has",
				"its own login, and downstream clusters' agents cannot sign in through",
				"Pangolin. You may still be logged into Pangolin in your browser, which",
				"can hide this — test with curl, not the browser.",
			}, ingressNote...),
			Check: func(ctx context.Context) error {
				if _, err := m.lookupHost(ctx, e.Hostname); err != nil {
					return fmt.Errorf("%s does not resolve yet: %w", e.Hostname, err)
				}
				return nil
			},
		},
	}
}

// Verify requests Rancher's /ping through the public hostname, retrying while
// the tunnel and Rancher settle, and turns the failure into the setup mistake
// it most likely points to.
func (m pangolinMode) Verify(ctx context.Context, e Exposure) error {
	fmt.Printf("    Checking https://%s/ping through the tunnel (up to %s)...\n", e.Hostname, m.timeout)
	deadline := time.Now().Add(m.timeout)
	var last error
	for {
		last = m.ping(ctx, e.Hostname)
		if last == nil {
			return nil
		}
		if errors.Is(last, errAuthRequired) {
			return last // retrying cannot fix a misconfigured resource
		}
		fmt.Printf("    not reachable yet: %v\n", last)
		if time.Now().Add(m.interval).After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.interval):
		}
	}
}

func (m pangolinMode) ping(ctx context.Context, host string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.pingURL(host), nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		var dnsErr *net.DNSError
		var certErr *x509.UnknownAuthorityError
		var hostErr *x509.HostnameError
		switch {
		case errors.As(err, &dnsErr):
			return fmt.Errorf("%s does not resolve — has the Pangolin resource been created? (%w)", host, err)
		case errors.As(err, &certErr), errors.As(err, &hostErr):
			return fmt.Errorf("certificate error for %s — the tunnel is not presenting a valid certificate for this domain (%w)", host, err)
		}
		return fmt.Errorf("could not reach https://%s: %w", host, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		if strings.TrimSpace(string(body)) != "pong" {
			return fmt.Errorf("https://%s/ping answered HTTP 200 but not with Rancher's \"pong\" — something else (an auth or error page?) is in front of Rancher", host)
		}
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return errAuthRequired
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Errorf("tunnel reached but nothing answered behind it (HTTP %d) — check the resource target (%s) and that the site is connected", resp.StatusCode, pangolinTarget)
	case http.StatusNotFound:
		return fmt.Errorf("tunnel answered HTTP 404 — the request did not reach Rancher's ingress for %s; check the resource target and that the Host header is preserved", host)
	default:
		return fmt.Errorf("unexpected HTTP %d from https://%s/ping", resp.StatusCode, host)
	}
}
