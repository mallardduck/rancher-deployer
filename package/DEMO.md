# Rancher Demo Image - User Guide

The `rancher-demo` Docker image provides a fully self-contained Rancher deployment running on k3s inside a single container. Perfect for testing, development, and demos.

---

## Quick Start

**Minimal deployment** (uses all defaults):

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  rancher-demo:latest
```

After ~2-3 minutes, Rancher will be accessible at `https://127.0.0.1.sslip.io`

**Get the bootstrap password:**

```bash
docker exec rancher-demo kubectl get secret --namespace cattle-system bootstrap-secret \
  -o go-template='{{.data.bootstrapPassword|base64decode}}{{"\n"}}'
```

---

## Environment Variables

Configure the deployment by setting environment variables with `-e` or `--env-file`:

| Variable | Default | Description |
|----------|---------|-------------|
| `RANCHER_VERSION` | `2.14.2` | Rancher version to deploy (e.g., `2.14.2`, `2.15.0`) |
| `RANCHER_INGRESS_ENABLED` | `false` | Enable ingress with hostname restrictions. When `false` (default), Rancher is accessible via any hostname/IP for flexible proxy setups. Set to `true` for ingress with specific hostname |
| `RANCHER_HOSTNAME` | Auto (IP.sslip.io) | Hostname for Rancher ingress (only used when `RANCHER_INGRESS_ENABLED=true`) |
| `RANCHER_BOOTSTRAP_PASSWORD` | `letsmein` | Initial admin password for Rancher |
| `RANCHER_NAMESPACE` | `cattle-system` | Kubernetes namespace for Rancher |
| `RANCHER_VALUES_FILE` | *(none)* | Path to Helm values YAML inside container |
| `RANCHER_HELM_SET` | *(none)* | Comma-separated `--set` values for Helm |
| `RANCHER_PRIME` | `false` | Set to `true` to use Rancher Prime edition |
| `RANCHER_CHANNEL` | `stable` | Release channel: `stable`, `latest`, `alpha` |
| `K8S_VERSION` | Auto-detected | Target k8s major.minor (e.g., `1.28`, `1.31`) |

---

## Ingress Configuration

By default, the rancher-demo image **disables Kubernetes ingress** (`RANCHER_INGRESS_ENABLED=false`). This matches the behavior of the original `rancher/rancher` Docker image and provides maximum flexibility:

**Benefits of disabled ingress (default):**
- ✅ Access Rancher via any hostname or IP address
- ✅ No hostname restrictions or DNS requirements
- ✅ Works seamlessly with external proxies (Traefik, nginx, Caddy, etc.)
- ✅ Simpler setup for Docker-based deployments
- ✅ No ingress resources created in Kubernetes

**When to enable ingress:**
- You want a specific hostname enforced at the Kubernetes level
- You're testing ingress-related features
- You prefer the k3d deployment style

To enable ingress:
```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_INGRESS_ENABLED=true \
  -e RANCHER_HOSTNAME=rancher.mylab.local \
  rancher-demo:latest
```

**Checking your setup:**
```bash
# With ingress disabled (default):
docker exec rancher-demo kubectl get ingress -A
# Output: No resources found

# With ingress enabled:
docker exec rancher-demo kubectl get ingress -A
# Output: Shows rancher ingress with your hostname
```

---

## Common Use Cases

### Custom Hostname and Bootstrap Password

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_HOSTNAME=rancher.homelab.local \
  -e RANCHER_BOOTSTRAP_PASSWORD=MySecurePass123 \
  rancher-demo:latest
```

Access at: `https://rancher.homelab.local`

---

### Using with Traefik or Other Reverse Proxies

**Default setup (recommended)** — No ingress, works with any proxy:

By default, `RANCHER_INGRESS_ENABLED=false`, which means Rancher is accessible via any hostname/IP. This is perfect for external proxies like Traefik, nginx, Caddy, etc.

```bash
docker run -d --privileged \
  --name rancher-demo \
  --network traefik-network \
  -e RANCHER_BOOTSTRAP_PASSWORD=MySecurePass \
  -l "traefik.enable=true" \
  -l "traefik.http.routers.rancher.rule=Host(\`rancher.example.com\`)" \
  -l "traefik.http.routers.rancher.entrypoints=websecure" \
  -l "traefik.http.routers.rancher.tls.certresolver=letsencrypt" \
  -l "traefik.http.services.rancher.loadbalancer.server.port=443" \
  -l "traefik.http.services.rancher.loadbalancer.server.scheme=https" \
  rancher-demo:latest
```

**Note:** Do not publish ports (`-p`) when using an external proxy — let the proxy handle routing. With ingress disabled (default), the external proxy can use any hostname without restrictions.

**Alternative with ingress enabled:**

If you prefer to use Rancher's built-in ingress:

```bash
docker run -d --privileged \
  --name rancher-demo \
  --network traefik-network \
  -e RANCHER_INGRESS_ENABLED=true \
  -e RANCHER_HOSTNAME=rancher.example.com \
  -e RANCHER_BOOTSTRAP_PASSWORD=MySecurePass \
  rancher-demo:latest
```

---

### Expose on Specific IP Address

If your Docker host has multiple IPs and you want to expose Rancher on a specific one:

```bash
# Bind to 192.168.1.100
docker run -d --privileged \
  --name rancher-demo \
  -p 192.168.1.100:80:80 \
  -p 192.168.1.100:443:443 \
  -e RANCHER_HOSTNAME=192.168.1.100.sslip.io \
  rancher-demo:latest
```

Access at: `https://192.168.1.100.sslip.io`

---

### Using a Custom Helm Values File

Create a `custom-values.yaml` file:

```yaml
replicas: 1
bootstrapPassword: "CustomPasswordHere"
auditLog:
  level: 1
ingress:
  tls:
    source: letsEncrypt
letsEncrypt:
  email: admin@example.com
  environment: production
```

Mount and use it:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -v $(pwd)/custom-values.yaml:/config/values.yaml:ro \
  -e RANCHER_VALUES_FILE=/config/values.yaml \
  rancher-demo:latest
```

---

### Helm --set Values (Inline Configuration)

Pass Helm chart values directly without a file:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_HELM_SET="replicas=3,auditLog.level=2,bootstrapPassword=SecurePass" \
  rancher-demo:latest
```

**Note:** Values are comma-separated. For complex configurations, use a values file instead.

---

### Rancher Prime Edition

Deploy Rancher Prime instead of the community edition:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_VERSION=2.14.2 \
  -e RANCHER_PRIME=true \
  rancher-demo:latest
```

---

### Using Latest/RC Versions

Test release candidates or alpha builds:

```bash
# Latest channel (includes RCs)
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_VERSION=2.15.0-rc1 \
  -e RANCHER_CHANNEL=latest \
  rancher-demo:latest

# Alpha channel
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_VERSION=2.15.0-alpha1 \
  -e RANCHER_CHANNEL=alpha \
  rancher-demo:latest
```

---

## Persistent Data

By default, the container stores k3s and Rancher data in volumes. To persist data across container restarts:

```bash
# Create a named volume
docker volume create rancher-data

# Use it
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -v rancher-data:/var/lib/rancher \
  rancher-demo:latest
```

**Restart behavior:**
- On restart with a persistent volume, the script detects the existing Rancher installation and skips redeployment
- To do a fresh deployment, delete the container and volume

---

## Accessing the Container

### Interactive Shell

```bash
docker exec -it rancher-demo bash
```

### View Logs

```bash
# Container logs (shows entrypoint output)
docker logs -f rancher-demo

# k3s logs
docker exec rancher-demo cat /var/log/k3s.log

# Rancher deployment logs
docker exec rancher-demo kubectl logs -n cattle-system -l app=rancher
```

### Using kubectl

```bash
# List all pods
docker exec rancher-demo kubectl get pods -A

# Get nodes
docker exec rancher-demo kubectl get nodes -o wide

# Describe Rancher deployment
docker exec rancher-demo kubectl describe deployment rancher -n cattle-system
```

### Using k9s (Interactive Cluster Manager)

The container includes `k9s` for interactive cluster exploration:

```bash
docker exec -it rancher-demo k9s
```

---

## Debugging Tools

The image includes built-in debug scripts:

### Debug Rancher

```bash
docker exec rancher-demo debug-rancher
```

Shows:
- Rancher pod status
- Recent logs
- Ingress configuration
- Service endpoints

### Debug cert-manager

```bash
docker exec rancher-demo debug-cert-manager
```

Shows:
- cert-manager pod status
- Certificate status
- Certificate request details
- Common cert-manager issues

---

## Advanced Configuration

### Using an .env File

Create a `.env` file:

```bash
RANCHER_VERSION=2.14.2
RANCHER_HOSTNAME=rancher.mylab.local
RANCHER_BOOTSTRAP_PASSWORD=SuperSecurePass123
RANCHER_HELM_SET=replicas=3,auditLog.level=1
```

Use it:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  --env-file .env \
  rancher-demo:latest
```

---

### Combining Environment Variables with Direct Args

Environment variables are processed first, then any additional `rancher-deployer` flags passed directly to the container are appended:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_VERSION=2.14.2 \
  rancher-demo:latest \
  --set ingress.tls.source=secret
```

This gives you flexibility to mix and match configuration methods.

---

## Troubleshooting

### Container exits immediately

**Check logs:**
```bash
docker logs rancher-demo
```

**Common causes:**
- Missing `--privileged` flag (required for k3s)
- Port conflicts (80/443 already in use)
- Invalid `RANCHER_VERSION`

---

### Rancher UI not accessible

**Wait for deployment:**
The initial deployment takes 2-4 minutes. Check progress:

```bash
docker logs -f rancher-demo
```

**Check Rancher pod status:**
```bash
docker exec rancher-demo kubectl get pods -n cattle-system -l app=rancher
```

**Run debug script:**
```bash
docker exec rancher-demo debug-rancher
```

---

### Certificate issues

**Check cert-manager:**
```bash
docker exec rancher-demo debug-cert-manager
```

**Manually check certificate:**
```bash
docker exec rancher-demo kubectl get certificate -n cattle-system
docker exec rancher-demo kubectl describe certificate -n cattle-system
```

---

### Hostname resolution

If using a custom hostname, ensure:
1. DNS resolves to the Docker host IP
2. Or add an entry to your `/etc/hosts`:
   ```
   127.0.0.1  rancher.homelab.local
   ```

For `sslip.io` domains, no DNS configuration is needed.

---

### Reset and Start Fresh

```bash
# Stop and remove container
docker rm -f rancher-demo

# Remove the volume (destroys all data)
docker volume rm rancher-data

# Start fresh
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -v rancher-data:/var/lib/rancher \
  rancher-demo:latest
```

---

## Docker Compose Example

Create a `docker-compose.yml`:

```yaml
services:
  rancher-demo:
    image: rancher-demo:latest
    container_name: rancher-demo
    privileged: true
    ports:
      - "80:80"
      - "443:443"
    environment:
      RANCHER_VERSION: "2.14.2"
      RANCHER_HOSTNAME: "rancher.homelab.local"
      RANCHER_BOOTSTRAP_PASSWORD: "MySecurePassword"
      RANCHER_HELM_SET: "replicas=1,auditLog.level=1"
    volumes:
      - rancher-data:/var/lib/rancher
      - rancher-kubelet:/var/lib/kubelet
      - rancher-cni:/var/lib/cni
      - rancher-logs:/var/log
    restart: unless-stopped

volumes:
  rancher-data:
  rancher-kubelet:
  rancher-cni:
  rancher-logs:
```

Run it:

```bash
docker compose up -d
```

---

## Limitations

- **Requires privileged mode** — k3s needs elevated privileges to run
- **Single-node only** — This is a demo environment, not production-ready
- **Resource usage** — Runs a full k3s cluster + Rancher; ensure adequate CPU/memory
- **Not suitable for production** — Use real k3s/k3d/RKE2 clusters for production workloads

---

## Getting Help

- **Check container logs:** `docker logs rancher-demo`
- **Debug scripts:** `debug-rancher` and `debug-cert-manager`
- **Interactive shell:** `docker exec -it rancher-demo bash`
- **View k9s:** `docker exec -it rancher-demo k9s`
- **Project repo:** https://github.com/mallardduck/rancher-deployer

---

## What's Included

The `rancher-demo` image contains:

- **k3s** (Kubernetes distribution)
- **Rancher** (multi-cluster management platform)
- **cert-manager** (automatic TLS certificates)
- **kubectl** (Kubernetes CLI)
- **helm** (Kubernetes package manager)
- **k9s** (interactive cluster management)
- **rancher-deployer** (CLI that orchestrates everything)

All running in a single, self-contained container.
