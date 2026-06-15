# Multi-Stage Dockerfile

Single Dockerfile with three build targets for layered image variants.

## Build Targets

### Target 1: `k3s-base`
**Purpose**: Minimal k3s-in-docker foundation  
**What it includes**: k3s runtime only  
**Use case**: Testing k3s versions, running custom workloads

```bash
docker build -f package/Dockerfile --target=k3s-base -t k3s-base:test .
docker run -d --privileged --name k3s-base -p 6443:6443 k3s-base:test
docker exec k3s-base kubectl get nodes
```

**What you get**: Working k3s cluster, nothing else

---

### Target 2: `k3s-tools`
**Purpose**: k3s-base + debugging tools  
**What it includes**: k3s + k9s + helm + debug scripts  
**Use case**: Interactive debugging, manual deployments

```bash
docker build -f package/Dockerfile --target=k3s-tools -t k3s-tools:test .
docker run -d --privileged --name k3s-tools k3s-tools:test
docker exec -it k3s-tools k9s
docker exec -it k3s-tools debug-cert-manager
```

**What you get**: k3s + tools to debug and manage it

---

### Target 3: `rancher-deployer` (default)
**Purpose**: Full Rancher deployment stack  
**What it includes**: k3s + tools + rancher-deployer binary  
**Use case**: Automated Rancher deployment

```bash
docker build -f package/Dockerfile -t rancher-deployer:test .
# Or explicitly:
docker build -f package/Dockerfile --target=rancher-deployer -t rancher-deployer:test .

docker run -d --privileged --name rancher \
  -p 80:80 -p 443:443 \
  -v rancher-data:/var/lib/rancher \
  -e RANCHER_VERSION=2.8.5 \
  rancher-deployer:test
```

**What you get**: k3s + Rancher deployed automatically

---

## Architecture

```
┌─────────────────────────────────────────┐
│ rancher-deployer (Target 3)             │
│ - Adds: rancher-deployer binary         │
│ - Entrypoint: deployment wrapper        │
├─────────────────────────────────────────┤
│ k3s-tools (Target 2)                    │
│ - Adds: k9s, helm, debug scripts        │
│ - Entrypoint: inherited from k3s-base   │
├─────────────────────────────────────────┤
│ k3s-base (Target 1)                     │
│ - Adds: k3s runtime setup                │
│ - Entrypoint: starts k3s server         │
├─────────────────────────────────────────┤
│ base (internal stage)                   │
│ - SUSE BCI 15.7 micro + packages        │
│ - Multi-stage build pattern             │
└─────────────────────────────────────────┘
```

## Environment Variables

### All Targets
- `CATTLE_K3S_VERSION` - K3s version to download (default: v1.28.5+k3s1)
- `KUBECONFIG` - Kubeconfig location (default: /etc/rancher/k3s/k3s.yaml)

### rancher-deployer Target Only
- `RANCHER_VERSION` - Rancher version to deploy (default: 2.8.5)
- `K3S_VERSION` - Override k3s version resolution

## Quick Start Testing

### Test k3s-base (Foundation)
```bash
# Build & run
docker build -f package/Dockerfile --target=k3s-base -t k3s-base:test .
docker run -d --privileged --name k3s-test -p 6443:6443 k3s-base:test

# Verify (wait ~30s for node to be Ready)
docker exec k3s-test kubectl get nodes
docker exec k3s-test kubectl run test --image=nginx
docker exec k3s-test kubectl get pods
```

**Success**: Node shows `Ready`, test pod shows `Running`

---

### Test k3s-tools (Foundation + Tools)
```bash
# Build & run
docker build -f package/Dockerfile --target=k3s-tools -t k3s-tools:test .
docker run -d --privileged --name k3s-tools-test k3s-tools:test

# Verify tools
docker exec -it k3s-tools-test k9s              # Interactive UI
docker exec k3s-tools-test helm version         # Should show v4.2.1
docker exec k3s-tools-test debug-cert-manager   # Cluster diagnostics
```

**Success**: All tools work, k3s healthy

---

### Test rancher-deployer (Full Stack)
```bash
# Build & run
docker build -f package/Dockerfile -t rancher-deployer:test .
docker run -d --privileged --name rancher-test \
  -p 80:80 -p 443:443 \
  -v rancher-data:/var/lib/rancher \
  -e RANCHER_VERSION=2.8.5 \
  rancher-deployer:test

# Monitor deployment (takes 5-10 minutes)
docker logs -f rancher-test

# Verify
docker exec rancher-test kubectl get pods -n cert-manager    # 3 Running
docker exec rancher-test kubectl get pods -n cattle-system   # Rancher Running
curl -k https://localhost/dashboard/                         # Rancher UI

# Get bootstrap password
docker exec rancher-test kubectl get secret --namespace cattle-system \
  bootstrap-secret -o go-template='{{.data.bootstrapPassword|base64decode}}{{"\n"}}'
```

**Success**: cert-manager and Rancher pods `Running`, UI accessible at https://localhost

---

## Common Debugging Scenarios

### k3s won't start
```bash
docker logs <container-id>                          # Check startup logs
docker exec -it <container-id> ps aux | grep k3s   # Verify k3s process
docker exec <container-id> cat /var/log/k3s.log    # Check k3s logs
```

### Pods stuck in Pending/ContainerCreating
```bash
docker exec <container-id> kubectl get pods -A                      # Check all pods
docker exec <container-id> kubectl get pods -n kube-system          # Check CNI (flannel)
docker exec <container-id> kubectl describe pod <pod> -n <ns>       # Details + events
docker exec -it <container-id> k9s                                  # Interactive debugging
```

### cert-manager timeout
```bash
docker exec <container-id> debug-cert-manager      # Run diagnostics
docker exec <container-id> kubectl get pods -n cert-manager -o wide
docker exec <container-id> kubectl get events -n cert-manager
```

### Network issues
```bash
docker exec <container-id> kubectl get pods -n kube-system | grep flannel
docker exec <container-id> ip addr
```

---

## Version Overrides

### Change K3s Version
```bash
docker run -d --privileged \
  -e CATTLE_K3S_VERSION=v1.29.0+k3s1 \
  k3s-base:test
```

### Change Rancher Version
```bash
docker run -d --privileged \
  -e RANCHER_VERSION=2.9.0 \
  rancher-deployer:test
```

### Pin Both Versions
```bash
docker run -d --privileged \
  -e CATTLE_K3S_VERSION=v1.29.0+k3s1 \
  -e RANCHER_VERSION=2.9.0 \
  rancher-deployer:test
```

## Additional K3s Flags

You can pass additional flags to k3s server:

```bash
docker run -d --privileged k3s-base:test \
  --disable=servicelb \
  --disable=local-storage \
  --cluster-cidr=10.43.0.0/16
```

## Multi-Architecture Builds

Build for specific architecture:
```bash
docker buildx build \
  -f package/Dockerfile \
  --target=k3s-base \
  --platform=linux/amd64 \
  -t k3s-base:amd64 .

docker buildx build \
  -f package/Dockerfile \
  --target=k3s-base \
  --platform=linux/arm64 \
  -t k3s-base:arm64 .
```

## Image Characteristics

| Aspect | Implementation |
|--------|----------------|
| **Structure** | Multi-stage build with multiple targets |
| **Base Image** | SUSE BCI 15.7 micro (minimal footprint) |
| **Package Install** | zypper via chroot pattern |
| **K3s Install** | Runtime download (version configurable) |
| **Build Targets** | k3s-base, k3s-tools, rancher-deployer |
| **Volumes** | /var/lib/rancher, /var/lib/kubelet, /var/lib/cni, /var/log |
| **Entrypoint** | k3s server (base/tools) or deployment wrapper (rancher-deployer) |

## File Structure

```
package/
├── Dockerfile                     # Multi-stage Dockerfile with 3 targets
├── k3s-entrypoint.sh              # k3s startup logic
├── rancher-entrypoint.sh          # Deployment wrapper
├── debug-cert-manager.sh          # Debug utilities
├── debug-rancher.sh               # Rancher diagnostics
└── README.md                      # This file
```

## Build Target Architecture

Multi-stage Dockerfile with three targets:
```bash
package/Dockerfile --target=k3s-base       # Minimal k3s runtime
package/Dockerfile --target=k3s-tools      # k3s + debugging tools
package/Dockerfile --target=rancher-deployer  # Full stack (default)
```

## Debugging

If a target fails, test the previous layer:

```
rancher-deployer fails?
  ↓
Test k3s-tools
  ↓
k3s-tools fails?
  ↓
Test k3s-base
  ↓
k3s-base fails?
  ↓
Check package/k3s-entrypoint.sh
```

Each layer builds on proven foundation, so failures are easy to isolate.

## Next Steps

1. ✅ Test k3s-base works (you already did this!)
2. ⏭️ Build and test k3s-tools
3. ⏭️ Build and test rancher-deployer
4. 🎉 Ship it
