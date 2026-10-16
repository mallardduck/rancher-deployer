package controller

import "github.com/mallardduck/rancher-deployer/internal/helm"

// Compile-time check that Controller satisfies helm.Backend. It lives in a
// test file so the non-test package never imports internal/helm.
var _ helm.Backend = (*Controller)(nil)
