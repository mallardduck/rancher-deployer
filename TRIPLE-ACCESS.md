# Triple Access Configuration

The `rancher-demo` Docker image provides **three access methods** to Rancher - all work simultaneously, giving you maximum flexibility.

## Overview

| Access Method | Ports | How It Works | Use When |
|---------------|-------|--------------|----------|
| **Localhost Ingress** | 80, 443 | Ingress configured for `localhost` hostname | Quick local testing (easiest!) |
| **Hostname Ingress** | 80, 443 | Traditional hostname-based routing via Traefik (sslip.io) | Network access, specific hostname needed |
| **Direct NodePort** | 8080, 8443 | Direct to Rancher service, bypasses ingress | External proxies, any hostname/IP, simplified routing |

## How It Works

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Docker Container: rancher-demo                              │
│                                                             │
│  Ports 80/443 (Traefik Ingress)                            │
│       ↓                                                     │
│  ┌───────────────────────┐    ┌─────────────────┐         │
│  │   Traefik Ingress     │───→│ Rancher Service │         │
│  │ 1. localhost          │    │   (ClusterIP)   │         │
│  │ 2. rancher.x.sslip.io │    └─────────────────┘         │
│  └───────────────────────┘             ↑                   │
│                                         │                   │
│  Ports 8080/8443 (NodePort)            │                   │
│  ┌─────────────────────┐               │                   │
│  │ rancher-nodeport    │───────────────┘                   │
│  │ Service (NodePort)  │                                   │
│  └─────────────────────┘                                   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Configuration Details

**Helm Values Set via extraObjects:**
```yaml
hostname: rancher.x.x.x.x.sslip.io  # Used by default ingress

extraObjects:
  # 1. Additional localhost ingress
  - apiVersion: networking.k8s.io/v1
    kind: Ingress
    metadata:
      name: rancher-localhost
      namespace: cattle-system
    spec:
      ingressClassName: traefik
      rules:
      - host: localhost
        http:
          paths:
          - path: /
            backend:
              service:
                name: rancher
                port:
                  number: 80

  # 2. NodePort service for direct access
  - apiVersion: v1
    kind: Service
    metadata:
      name: rancher-nodeport
      namespace: cattle-system
    spec:
      type: NodePort
      selector:
        app: rancher
      ports:
      - port: 80
        nodePort: 30080
      - port: 443
        nodePort: 30443
```

**Kubernetes Resources Created:**
```bash
# Two ingresses: default hostname + localhost
$ kubectl get ingress -n cattle-system
NAME                CLASS     HOSTS                           ADDRESS      PORTS     AGE
rancher             traefik   rancher.192.168.1.10.sslip.io   10.43.0.5    80, 443   5m
rancher-localhost   traefik   localhost                       10.43.0.5    80, 443   5m

# Default service (ClusterIP) used by both ingresses
$ kubectl get svc -n cattle-system rancher
NAME      TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)      AGE
rancher   ClusterIP   10.43.45.67    <none>        80/TCP,443/TCP   5m

# Additional NodePort service for direct access
$ kubectl get svc -n cattle-system rancher-nodeport
NAME               TYPE       CLUSTER-IP     EXTERNAL-IP   PORT(S)                      AGE
rancher-nodeport   NodePort   10.43.12.34    <none>        80:30080/TCP,443:30443/TCP   5m
```

**Key Insight:** We use `extraObjects` to inject additional resources (localhost ingress + NodePort service) without modifying the Rancher chart's default behavior!

## Usage Examples

### Standard Usage (All Three Methods Available)

```bash
docker run -d --privileged \
  --name rancher-demo \
  -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 \
  rancher-demo:latest
```

**Method 1: Access via localhost ingress (easiest!):**
```bash
curl -k https://localhost
# Just works! No IP address or special hostname needed
```

**Method 2: Access via hostname ingress:**
```bash
# Uses auto-detected IP with sslip.io
curl -k https://rancher.127.0.0.1.sslip.io
```

**Method 3: Access via NodePort (any hostname/IP):**
```bash
# Works with any hostname
curl -k https://localhost:8080
curl -k https://rancher.localhost:8080
curl -k https://foo.bar:8080  # All work!
```

### With External Reverse Proxy (Traefik, nginx, etc.)

**Recommended: Route to NodePort (8443)**

```bash
docker run -d --privileged \
  --name rancher-demo \
  --network proxy-network \
  -e RANCHER_BOOTSTRAP_PASSWORD=SecurePass \
  -l "traefik.enable=true" \
  -l "traefik.http.routers.rancher.rule=Host(\`rancher.example.com\`)" \
  -l "traefik.http.routers.rancher.entrypoints=websecure" \
  -l "traefik.http.routers.rancher.tls.certresolver=letsencrypt" \
  -l "traefik.http.services.rancher.loadbalancer.server.port=8443" \
  -l "traefik.http.services.rancher.loadbalancer.server.scheme=https" \
  rancher-demo:latest
```

**Why port 8443?**
- Works with **any** hostname your proxy sends
- No hostname validation conflicts
- Simpler routing

**Alternative: Route to ingress (443)**
```yaml
traefik.http.services.rancher.loadbalancer.server.port=443
```
- Uses Rancher's built-in ingress
- Hostname must match Rancher configuration
- Traditional setup

## Comparison with Old rancher/rancher Image

### Old `rancher/rancher` Behavior

```bash
docker run -d --privileged -p 80:80 -p 443:443 rancher/rancher:v2.8.5
# No ingress created
# Direct service access only
# Works with any hostname/IP
```

### New `rancher-demo` Behavior

```bash
docker run -d --privileged -p 80:80 -p 443:443 -p 8080:8080 -p 8443:8443 rancher-demo:latest
# Ingress created (ports 80/443)
# NodePort available (ports 8080/8443)
# Best of both worlds!
```

## Benefits

### ✅ **Maximum Flexibility**
- Choose the access method that works best for your setup
- No configuration needed - both methods always available

### ✅ **Backward Compatible**
- NodePort access (8080/8443) provides old rancher/rancher behavior
- Ingress access (80/443) provides traditional Rancher experience

### ✅ **Perfect for External Proxies**
- Use NodePort (8080/8443) with any external proxy
- No hostname conflicts or validation issues
- Simplified routing

### ✅ **No Trade-offs**
- Both methods work simultaneously
- No need to choose between ingress and direct access
- Full Rancher functionality with both methods

## Port Reference

| Port | Purpose | Access Method | Hostname | Notes |
|------|---------|---------------|----------|-------|
| 80 | HTTP ingress | Via Traefik | `localhost` OR `rancher.x.sslip.io` | Two ingresses handle both |
| 443 | HTTPS ingress | Via Traefik | `localhost` OR `rancher.x.sslip.io` | Two ingresses handle both |
| 8080 | HTTP NodePort | Direct to service | Any hostname/IP | No validation |
| 8443 | HTTPS NodePort | Direct to service | Any hostname/IP | No validation |
| 6443 | Kubernetes API | k3s | N/A | Internal use |

## Technical Notes

### Why Set service.type=NodePort?

1. **Enables dual access**: NodePort services provide both ClusterIP and NodePort endpoints
2. **Works with ingress**: Traefik routes to the ClusterIP (not affected by NodePort)
3. **No conflicts**: Both access methods coexist peacefully

### Port Mapping in k3s Container

```
Docker Host         Container (k3s)           Kubernetes
-----------         ----------------          -----------
80        →         30080 (Traefik)    →     Rancher via Ingress
443       →         30443 (Traefik)    →     Rancher via Ingress
8080      →         30080 (NodePort)   →     Rancher Service Direct
8443      →         30443 (NodePort)   →     Rancher Service Direct
```

Note: Traefik in k3s is configured to use host ports 80/443, which map to the NodePorts.

## Troubleshooting

### "Connection refused" on port 8080

**Check NodePort is configured:**
```bash
docker exec rancher-demo kubectl get svc -n cattle-system rancher -o yaml | grep -A 2 "type:"
# Should show: type: NodePort
```

**Verify Helm values:**
```bash
docker exec rancher-demo helm get values rancher -n cattle-system
# Should include: service.type=NodePort
```

### Ingress not working

**Check ingress exists:**
```bash
docker exec rancher-demo kubectl get ingress -n cattle-system
# Should show rancher ingress
```

**Verify hostname:**
```bash
docker exec rancher-demo kubectl get ingress -n cattle-system rancher -o jsonpath='{.spec.rules[0].host}'
# Shows configured hostname
```

## Summary

The triple access approach gives you maximum flexibility:
1. **`https://localhost`** (80/443) - Easiest! Just works for local testing
2. **`https://rancher.<ip>.sslip.io`** (80/443) - Network access with proper hostname
3. **`https://localhost:8080`** (8080/8443) - Direct NodePort, works with any hostname/IP

No configuration needed - all three methods work out of the box! 🎉

**Pro tip:** For most users, just use `https://localhost` and you're done!
