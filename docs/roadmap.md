# Future Ideas

Add things here when users actually ask for them.

## Doctor Command: Auto-Fix Missing Dependencies

Enhance the `doctor` command to automatically download missing binaries using [github.com/mallardduck/ghreleases](https://github.com/mallardduck/ghreleases).

### Goals
- **Self-healing**: Running `rancher-deployer doctor --fix` automatically downloads missing dependencies
- **Local bin directory**: Store fetched binaries in `.bin/` or `~/.rancher-deployer/bin/` (added to PATH)
- **Version management**: Define logical rules for required versions
  - Track "latest" for components where it makes sense (kubectl, helm)
  - Pin specific versions for others (k3s/k3d based on Rancher compatibility)
- **Platform-aware**: Download correct binary for current OS/arch (darwin/arm64, linux/amd64, etc.)

### Implementation Approach
1. Create `internal/binman/` (binary manager) package
2. Define version requirements in config/code:
   ```go
   type BinarySpec struct {
       Name        string
       Source      string  // GitHub repo (e.g., "kubernetes/kubernetes")
       Version     string  // "latest" or specific version
       AssetMatch  string  // Regex to match release asset name
       Executable  string  // Name of binary in archive
   }
   ```
3. Integrate with `ghreleases` for downloading from GitHub releases
4. Add `--fix` flag to doctor command
5. Update PATH or use wrapper to execute from local bin directory

### Example Workflow
```bash
$ rancher-deployer doctor
  ✗ kubectl: not found in PATH
  ✗ helm: not found in PATH
  ⚠ Run 'rancher-deployer doctor --fix' to download missing dependencies

$ rancher-deployer doctor --fix
  Downloading kubectl v1.34.3 for darwin/arm64...
  Downloading helm v3.15.2 for darwin/arm64...
  ✔ All dependencies installed to ~/.rancher-deployer/bin/
  Add to PATH: export PATH="$HOME/.rancher-deployer/bin:$PATH"
```

### Binaries to Support
- **kubectl** - Latest stable from kubernetes/kubernetes releases
- **helm** - Latest stable from helm/helm releases
- **k3d** - Latest from k3d-io/k3d releases
- **k3s** (maybe) - Specific version based on Rancher compatibility matrix
- Future: cert-manager, rancher CLI, etc.

### Benefits
- Lower barrier to entry (no manual binary installation)
- Consistent versions across team members
- Works in CI/CD environments
- Prepares for air-gapped installation support

---

## If requested
- `--verbose` flag for debugging
- Shell completion (bash/zsh/fish)
- Remote deployment over SSH
- Air-gapped installation support

That's it. Everything else: build it when you need it.
