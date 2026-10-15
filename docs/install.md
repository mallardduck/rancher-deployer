# Installation

## From a GitHub release (recommended)

Prebuilt binaries for `linux`/`darwin` on `amd64`/`arm64` are published on the
[Releases page](https://github.com/mallardduck/rancher-deployer/releases), each with a
`rancher-deployer_<version>_checksums.txt` for verification.

```bash
# Set these for your platform, e.g. os=darwin arch=arm64
os=linux
arch=amd64
version=$(curl -fsSL https://api.github.com/repos/mallardduck/rancher-deployer/releases/latest | grep -m1 '"tag_name"' | cut -d'"' -f4)

curl -fsSL -o rancher-deployer.tar.gz \
  "https://github.com/mallardduck/rancher-deployer/releases/download/${version}/rancher-deployer_${version#v}_${os}_${arch}.tar.gz"
tar -xzf rancher-deployer.tar.gz rancher-deployer
sudo install -m 0755 rancher-deployer /usr/local/bin/rancher-deployer
```

Once installed, keep it up to date with the built-in updater, which checks the same
GitHub releases and verifies the downloaded binary against the published checksums:

```bash
rancher-deployer self-update          # check, prompt, and apply if available
rancher-deployer self-update --check  # just check
```

## From source (requires Go 1.25+)

```bash
git clone https://github.com/mallardduck/rancher-deployer
cd rancher-deployer
make build          # produces ./rancher-deployer
make install        # installs to $GOPATH/bin
```

## Cross-compile

```bash
make build-all      # builds linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
ls dist/
```

## Prerequisites

The binary shells out to these tools — they must be in `PATH` on the target node:

| Tool      | Required for        |
|-----------|---------------------|
| `helm`    | Always              |
| `kubectl` | Always              |
| `k3d`     | k3d mode (auto-installed if missing) |
| `k3s`     | k3s mode (installed via official script) |
| `curl`    | k3s install script  |
