# Helm values

Values are applied in this order (last wins):

1. Rancher chart defaults
2. `--values-file <file>` — your base values YAML
3. `--set key=value` flags — individual overrides

The `hostname` value is always injected automatically unless you set it yourself via `--hostname` or `--set hostname=...`. Some [`--expose` modes](expose.md) also add values (for example `--expose pangolin` adds `tls=external`); an explicit `--set` for the same key always wins.

**Example `values.yaml`:**

```yaml
replicas: 3
bootstrapPassword: "change-me-on-first-login"
auditLog:
  level: 1
ingress:
  tls:
    source: letsEncrypt
letsEncrypt:
  email: admin@example.com
  environment: production
```
