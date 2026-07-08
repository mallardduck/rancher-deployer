# Ingress Configuration Examples

This document shows how the new `--disable-ingress` flag works for both direct `rancher-deployer` usage and the Docker demo image.

## Background

The new ingress configuration provides maximum flexibility:

- **Default for k3d/k3s users**: Ingress **enabled** (no change from before)
- **Default for Docker image**: Ingress **disabled** (matches old `rancher/rancher` behavior)

## CLI Usage (`rancher-deployer` directly)

### With Ingress (Default - No Change)

```bash
# Traditional k3d deployment - ingress enabled by default
rancher-deployer deploy --rancher-version 2.14.2

# Result: Creates ingress with hostname restriction
kubectl get ingress -A
# NAMESPACE       NAME      CLASS     HOSTS                        ADDRESS      PORTS     AGE
# cattle-system   rancher   traefik   rancher.192.168.1.10.sslip.io   10.0.0.1   80, 443   5m
```

### Without Ingress (Opt-in for Docker-like behavior)

```bash
# Docker-style deployment - no ingress, direct service access
rancher-deployer deploy --rancher-version 2.14.2 --disable-ingress

# Result: No ingress created, accessible via any hostname
kubectl get ingress -A
# No resources found
```

## Docker Image Usage

### Default: No Ingress (Like Old rancher/rancher)

```bash
# Default behavior - no ingress created
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  rancher-demo:latest

# Access via ANY hostname or IP:
# https://localhost
# https://192.168.1.100
# https://rancher.homelab.local
# All work without DNS/hostname restrictions
```

Check inside container:
```bash
docker exec rancher-demo kubectl get ingress -A
# No resources found
```

### With Ingress (Opt-in for k3d-like behavior)

```bash
# Opt-in to ingress with hostname restrictions
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 \
  -e RANCHER_INGRESS_ENABLED=true \
  -e RANCHER_HOSTNAME=rancher.example.com \
  rancher-demo:latest

# Access ONLY via the specified hostname:
# https://rancher.example.com
```

Check inside container:
```bash
docker exec rancher-demo kubectl get ingress -A
# NAMESPACE       NAME      CLASS     HOSTS                   ADDRESS      PORTS     AGE
# cattle-system   rancher   traefik   rancher.example.com     172.17.0.5   80, 443   5m
```

## Use Cases

### Use Case 1: Traefik/nginx/Caddy Reverse Proxy

**Problem**: External proxy needs to route to Rancher, but ingress hostname restricts access.

**Solution**: Use default (ingress disabled):

```bash
docker run -d --privileged \
  --name rancher-demo \
  --network proxy-network \
  -e RANCHER_BOOTSTRAP_PASSWORD=SecurePass \
  rancher-demo:latest
# No -p flags needed, proxy handles routing
# Works with ANY hostname the proxy sends
```

### Use Case 2: Testing Behind Corporate Proxy

**Problem**: Corporate proxy modifies Host headers, causing ingress to reject requests.

**Solution**: Disable ingress (default):

```bash
docker run -d --privileged \
  -p 443:443 \
  rancher-demo:latest
# Access works regardless of proxy header modifications
```

### Use Case 3: Multi-hostname Access

**Problem**: Need to access Rancher from both localhost and LAN IP.

**Solution**: Use default (ingress disabled):

```bash
docker run -d --privileged \
  -p 80:80 -p 443:443 \
  rancher-demo:latest

# All of these work:
# https://localhost
# https://127.0.0.1
# https://192.168.1.100
# https://rancher.homelab.local
```

### Use Case 4: Testing Ingress Features

**Problem**: Need to test ingress-specific functionality.

**Solution**: Enable ingress:

```bash
docker run -d --privileged \
  -p 80:80 -p 443:443 \
  -e RANCHER_INGRESS_ENABLED=true \
  -e RANCHER_HOSTNAME=rancher.test.local \
  rancher-demo:latest
# Ingress created, can test ingress controllers, annotations, etc.
```

## Technical Details

### What Happens When Ingress is Disabled?

When `--disable-ingress` is used (or `RANCHER_INGRESS_ENABLED=false` in Docker):

1. Helm value set: `ingress.enabled=false`
2. Helm value set: `tls=external`
3. No Kubernetes ingress resource created
4. Rancher service accepts connections on all interfaces
5. No hostname validation at the ingress layer
6. External proxies can route traffic using any hostname

### What Happens When Ingress is Enabled?

When ingress is enabled (default for CLI, opt-in for Docker):

1. Kubernetes ingress resource created
2. Hostname restriction applied
3. Requests must have matching Host header
4. Works like traditional k3d deployments

## Migration from Old rancher/rancher Image

If you were using the old `rancher/rancher` Docker image:

**Old command:**
```bash
docker run -d --privileged -p 80:80 -p 443:443 rancher/rancher:v2.8.5
```

**New equivalent:**
```bash
docker run -d --privileged -p 80:80 -p 443:443 rancher-demo:latest
```

No changes needed! The default behavior matches the old image.
