// Package selfupdate wires binup against rancher-deployer's GitHub releases.
package selfupdate

import (
	"time"

	"github.com/mallardduck/binup"
	binupgithub "github.com/mallardduck/binup/github"
	"github.com/mallardduck/rancher-deployer/internal/version"
)

// repo is the GitHub repository releases are published to.
const repo = "mallardduck/rancher-deployer"

// Option configures New.
type Option func(*config)

type config struct{ prereleases bool }

// WithPrereleases opts in to prerelease builds. Without it, only stable
// releases are offered, so a stable install never moves to a prerelease by
// accident, while a prerelease install still moves to the stable that
// supersedes it.
func WithPrereleases(include bool) Option {
	return func(c *config) { c.prereleases = include }
}

// New builds a binup.Updater configured for rancher-deployer's GitHub
// releases. The asset and checksum-manifest templates match the naming
// goreleaser produces (see .goreleaser.yaml): versions are unprefixed
// ("0.3.1", not "v0.3.1"), even though GitHub tags carry the "v". Strict
// template mode turns a typo in either hand-written template into an
// immediate error instead of a confusing "asset not found" later.
func New(opts ...Option) (*binup.Updater, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	src := binupgithub.New(repo,
		binupgithub.WithPrereleases(cfg.prereleases),
		binupgithub.WithAssetTemplate("{name}_{version|trimprefix:v}_{os}_{arch}.{ext}"),
		binupgithub.WithChecksumAsset("{name}_{version|trimprefix:v}_checksums.txt"),
		binupgithub.WithTemplateMode(binupgithub.TemplateStrict),
	)
	return binup.New(src, binup.WithVersionProvider(binup.StaticVersionInfo(currentVersionInfo())))
}

// currentVersionInfo builds the VersionInfo binup reports as "current" from
// the ldflags-injected version package. version.Date is set via
// -ldflags "-X .../version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)", i.e. RFC3339;
// a dev build leaves it as the literal "unknown", which fails to parse — that's
// fine to ignore since BuildDate is display-only metadata, not used for the
// update decision.
func currentVersionInfo() binup.VersionInfo {
	info := binup.VersionInfo{
		Version: version.Version,
		Commit:  version.Commit,
	}
	if t, err := time.Parse(time.RFC3339, version.Date); err == nil {
		info.BuildDate = t
	}
	return info
}
