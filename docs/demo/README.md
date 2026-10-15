# Rancher Demo Image - User Guide

The `rancher-demo` Docker image provides a fully self-contained Rancher deployment running on k3s inside a single container. Perfect for testing, development, and demos.

---

## Quick Start

**Minimal deployment** (uses all defaults):

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 -p 8080:8080 -p 8443:8443 \
  rancher-demo:latest
```

After ~2-3 minutes, Rancher will be accessible at:
- **`https://localhost`** ← Easiest! Just works.
- `https://rancher.127.0.0.1.sslip.io` ← Full hostname
- `https://localhost:8080` ← Direct NodePort (works with any IP)

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
| `RANCHER_HOSTNAME` | Auto (IP.sslip.io) | Hostname for Rancher ingress |
| `RANCHER_BOOTSTRAP_PASSWORD` | `letsmein` | Initial admin password for Rancher |
| `RANCHER_NAMESPACE` | `cattle-system` | Kubernetes namespace for Rancher |
| `RANCHER_VALUES_FILE` | *(none)* | Path to Helm values YAML inside container |
| `RANCHER_HELM_SET` | *(none)* | Comma-separated `--set` values for Helm (Note: `service.type=NodePort` is set automatically) |
| `RANCHER_PRIME` | `false` | Set to `true` to use Rancher Prime edition |
| `RANCHER_CHANNEL` | `stable` | Release channel: `stable`, `latest`, `alpha` |
| `K8S_VERSION` | Auto-detected | Target k8s major.minor (e.g., `1.28`, `1.31`) |

---

## Multiple Access Methods

The rancher-demo image provides **three ways to access Rancher** - all work simultaneously:

### 1. **Localhost Ingress - `https://localhost` (Easiest!)**

Direct access via localhost ingress (no IP address needed):
- ✅ Just works! No configuration needed
- ✅ No need to know your IP address
- ✅ No sslip.io domain needed
- ✅ Standard HTTPS on ports 80/443
- ✅ Access: `https://localhost`

**This is the recommended method for most users!**

### 2. **Hostname-Based Ingress - `https://rancher.<ip>.sslip.io`**

Traditional Rancher ingress with auto-detected hostname:
- ✅ Uses your actual IP address
- ✅ Hostname-based routing via Traefik
- ✅ Good for accessing from other machines on your network
- ✅ Access: `https://rancher.192.168.1.100.sslip.io` (example)

### 3. **Direct NodePort - `https://localhost:8080`**

Direct access to the Rancher service (bypasses ingress):
- ✅ Works with **any** hostname or IP address
- ✅ Perfect for external reverse proxies (Traefik, nginx, etc.)
- ✅ No hostname validation
- ✅ Access: `https://localhost:8080`, `https://192.168.1.100:8080`, etc.

### Which One Should I Use?

**For quick local testing:** Use `https://localhost` (method #1)

**For network access:** Use `https://rancher.<ip>.sslip.io` (method #2)

**For external proxies:** Use NodePort on 8080/8443 (method #3)

**Checking your setup:**
```bash
# Two ingresses: main hostname + localhost
docker exec rancher-demo kubectl get ingress -n cattle-system
# Shows:
# - rancher (with sslip.io hostname)
# - rancher-localhost (with localhost)

# NodePort service for direct access
docker exec rancher-demo kubectl get svc -n cattle-system rancher-nodeport
# Shows TYPE=NodePort on ports 8080/8443
```

---

## Common Use Cases

### Custom Hostname and Bootstrap Password

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 -p 8080:8080 -p 8443:8443 \
  -e RANCHER_HOSTNAME=rancher.homelab.local \
  -e RANCHER_BOOTSTRAP_PASSWORD=MySecurePass123 \
  rancher-demo:latest
```

Access via:
- **Ingress**: `https://rancher.homelab.local`
- **NodePort**: `https://localhost:8080` (or any IP:8080)

---

### Using with Traefik or Other Reverse Proxies

**Recommended: Use NodePort (port 8443) for external proxies**

The NodePort access (8443) works with any hostname, making it perfect for external proxies:

```bash
docker run -d --privileged \
  --name rancher-demo \
  --network traefik-network \
  -e RANCHER_BOOTSTRAP_PASSWORD=MySecurePass \
  -l "traefik.enable=true" \
  -l "traefik.http.routers.rancher.rule=Host(\`rancher.example.com\`)" \
  -l "traefik.http.routers.rancher.entrypoints=websecure" \
  -l "traefik.http.routers.rancher.tls.certresolver=letsencrypt" \
  -l "traefik.http.services.rancher.loadbalancer.server.port=8443" \
  -l "traefik.http.services.rancher.loadbalancer.server.scheme=https" \
  rancher-demo:latest
```

**Key points:**
- Use port **8443** (NodePort) instead of 443 for external proxies
- No need to publish ports (`-p`) when using external proxy
- Works with **any** hostname your proxy sends
- Alternative: Use port 443 if you want the proxy to use Rancher's ingress

---

### Expose on Specific IP Address

If your Docker host has multiple IPs and you want to expose Rancher on a specific one:

```bash
# Bind to 192.168.1.100
docker run -d --privileged \
  --name rancher-demo \
  -p 192.168.1.100:80:80 \
  -p 192.168.1.100:443:443 \
  -p 192.168.1.100:8080:8080 \
  -p 192.168.1.100:8443:8443 \
  -e RANCHER_HOSTNAME=192.168.1.100.sslip.io \
  rancher-demo:latest
```

Access via:
- **Ingress**: `https://192.168.1.100.sslip.io`
- **NodePort**: `https://192.168.1.100:8080`

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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
  -e RANCHER_VERSION=2.15.0-rc1 \
  -e RANCHER_CHANNEL=latest \
  rancher-demo:latest

# Alpha channel
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
  --env-file .env \
  rancher-demo:latest
```

---

### Combining Environment Variables with Direct Args

Environment variables are processed first, then any additional `rancher-deployer` flags passed directly to the container are appended:

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
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
      - "80:80"       # Traefik ingress HTTP
      - "443:443"     # Traefik ingress HTTPS
      - "8080:8080"   # Direct NodePort HTTP
      - "8443:8443"   # Direct NodePort HTTPS
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
