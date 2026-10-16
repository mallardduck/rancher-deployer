# Exposing Rancher (`--expose`)

`--expose` chooses how Rancher is reached from outside the cluster. The mode
decides the Rancher hostname, any extra Helm values it needs, and any manual
setup you have to do (shown as steps before anything is installed).

| Mode      | Hostname                                 | Use when |
|-----------|------------------------------------------|----------|
| `sslip` *(default)* | `rancher.<node-ip>.sslip.io`, or `--hostname` | Local or LAN access; nothing to set up |
| `pangolin` | **`--hostname` is required**            | Rancher is published through a [Pangolin](https://github.com/fosrl/pangolin) tunnel on a domain you control |

`ngrok` is not implemented yet — see the [roadmap](../roadmap.md#--expose-ngrok).

`--hostname` (or `--set hostname=...`) always wins over what a mode would
derive. `--dry-run` prints a mode's setup steps and cluster config without
prompting or changing anything.

## `sslip` (default)

Behaves as before: the hostname is `rancher.<outbound-ip>.sslip.io`. If the
node IP can't be detected, it warns and uses `127.0.0.1`.

## `pangolin`

```bash
rancher-deployer deploy \
  --rancher-version 2.12 \
  --expose pangolin \
  --hostname rancher.example.com
```

`--hostname` is the public FQDN of your Pangolin resource. Pangolin's CLI
doesn't expose it, so the tool can't discover it. It must be a bare hostname
(no `https://`, no path).

### What the tool sets for you

- **`tls=external`** on the Rancher chart — Pangolin terminates TLS, so Rancher
  serves plain HTTP behind it. Your own `--set tls=...` takes precedence.
- **On k3s and k3d:** a Traefik `HelmChartConfig` that trusts forwarded headers
  on the `web` entrypoint (`forwardedHeaders.insecure: true`). Without it,
  Traefik replaces the tunnel's `X-Forwarded-Proto: https` with `http`, and
  Rancher redirects to HTTPS forever. This setting trusts `X-Forwarded-*` from
  any client that can reach port 80 on the node, which is acceptable for the
  dev and demo setups this tool targets but worth knowing.
- **On existing clusters:** nothing is changed. Your ingress controller must pass
  the tunnel's `X-Forwarded-Proto` through to Rancher, or you'll see the same
  redirect loop.

### The flow

After you confirm the plan, and before the cluster is installed, you're walked
through two steps (`--yes` shows them but doesn't wait):

1. **Start the tunnel** — create a Site in Pangolin and run the connect command
   it shows on this machine. Keep it running; use a second terminal if it runs
   in the foreground.
2. **Create the public resource** — in Pangolin, add a Resource with:
   - **Domain:** your `--hostname`
   - **Site:** the site from step 1
   - **Target:** `http://127.0.0.1:80` (k3s and k3d publish the ingress on host
     port 80). Rancher isn't installed yet, so expect a gateway error until it is.

   The tool checks that the hostname resolves in DNS before continuing.

After Rancher is ready, it requests `https://<hostname>/ping` through the tunnel
and reports the likely cause if that fails.

### Leave Pangolin authentication off

Rancher has its own login, and downstream clusters' agents can't sign in through
Pangolin's SSO. A resource with Pangolin authentication enabled will block
registration of downstream clusters, even though the UI may look fine in your
browser if you're logged into Pangolin. The final check reports a 401 or 403
from the tunnel as exactly this problem.

**Test with `curl`, not the browser:**

```bash
curl -sI https://rancher.example.com/ping
```

### Troubleshooting the final check

| Result | Likely cause |
|--------|--------------|
| hostname does not resolve | The Pangolin resource hasn't been created, or DNS hasn't propagated |
| 401 / 403 | Pangolin authentication (SSO) is enabled on the resource |
| 502 / 503 / 504 | The target address or port is wrong, or the site isn't connected |
| 404 | The request didn't reach Rancher's ingress for this hostname; check the target and that the `Host` header is preserved |
| certificate error | The tunnel isn't presenting a valid certificate for the domain |
| redirect loop in the browser | The ingress is overriding `X-Forwarded-Proto` (see above) |

A failed check is a warning, not an error — Rancher is installed either way.
