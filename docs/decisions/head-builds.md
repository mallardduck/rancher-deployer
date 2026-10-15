# Head Build Support

**Date:** 2026-09-03
**Status:** Implemented

## What "head builds" means here

Rancher continuously publishes unreleased "head" builds off its
`release-<minor>` branches — most relevantly, whatever the current `main`
branch minor is before it has a stable release. `rancher-deployer` supports
installing these via a new `head` release channel (`--channel head`), always
scoped to a bare minor (`--rancher-version 2.15`, not an exact patch — head
build hashes aren't user-guessable ahead of time).

## The community vs. Prime asymmetry

This is the part worth writing down: it's non-obvious, and wasn't documented
anywhere upstream at the time this was built. It was only found by probing
the live endpoints directly (commands to reproduce that are at the bottom of
this doc).

### Community

Head builds live in a dedicated, per-minor Helm repo:

```
https://charts.optimus.rancher.io/server-charts/release-<minor>
```

e.g. `release-2.15`. This is a different host+path pattern than community's
`stable`/`latest`/`alpha` channels (all on `releases.rancher.com`). As
observed live, this repo's `index.yaml` typically contains exactly one entry
at a time (continuously overwritten as new commits land), version-formatted
as `<minor>-<git-sha>-head` — no patch component — e.g.
`2.15-9f0d0301586ef2c690062d3a42bb3a91edfd3e12-head`.

### Prime

Prime head builds are **not** on a separate path. They're already mixed into
the existing Prime `latest` repo
(`https://charts.optimus.rancher.io/server-charts/latest`), interleaved with
GA and RC entries for every minor at once. As observed live, that single
index had roughly 974 entries spanning several minors' worth of head/RC/GA
builds simultaneously, with multiple head builds per minor (each a fresh git
commit). Prime head version strings do carry a patch component — e.g.
`2.16.0-b03c4de-head`.

Guessed Prime-specific head paths (`prime-release-<minor>` on both
`charts.optimus.rancher.io` and `charts.rancher.com`) do not exist.

### Why this matters for resolution

Head build version strings carry a git hash instead of a sortable sequence —
`2.16.0-b03c4de-head` vs. `2.16.0-de47bc4-head` have no defined ordering — so
"newest" can't be determined from the version string. Both paths instead sort
by each Helm index entry's `created` timestamp (see `resolveHeadEntry` in
`internal/rancher/rancher.go`).

Because Prime's shared `latest` index mixes head builds in with GA/RC — and a
head build's patch number can sort *above* the current GA (observed live:
`2.15.2-fbf2130-head` next to the actual `2.15.1` GA) — any non-head
resolution against that index (`resolveMinorEntry`, `FetchLatestVersion`)
must explicitly exclude `-head`-suffixed entries. Without that exclusion, a
plain `--channel latest` resolution could silently land on a head build.

## KDM data for brand-new minors

A brand-new minor (2.16, at the time this was written — no GA/RC release at
all yet) often has no dedicated KDM appVersion range either. In practice this
hasn't been a real blocker: Rancher's KDM data tends to widen an existing
range (observed: `>= 2.15.0-0 < 2.16.100-0`) to cover the new minor rather
than adding a dedicated one, and the `dev-v<minor>` GitHub branch fallback
(`internal/kdm/kdm.go`) tends to exist before the minor's Helm chart does. As
defense-in-depth for the rarer case where neither is true yet,
`kdm.FetchSupportMatrixWithFallback` falls back to the previous minor's KDM
data exactly once (`previousMinorVersion`), warning the caller when it does.

## Re-verifying this later

If Rancher's release infrastructure changes these paths, the quickest way to
check is:

```sh
# Community head (dedicated per-minor repo)
curl -s https://charts.optimus.rancher.io/server-charts/release-<minor>/index.yaml

# Prime head (mixed into the shared "latest" repo — look for "-head" entries)
curl -s https://charts.optimus.rancher.io/server-charts/latest/index.yaml | grep -- '-head'

# KDM primary / GitHub dev-branch fallback
curl -s https://releases.rancher.com/kontainer-driver-metadata/release-v<minor>/data.json
curl -s https://raw.githubusercontent.com/rancher/kontainer-driver-metadata/refs/heads/dev-v<minor>/data/data.json
```
