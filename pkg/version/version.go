// Package version holds the build-time version stamp shared by the
// bastion and daemon binaries.
package version

// Version is the semver tag the binary was built from, injected at build
// time via: -ldflags "-X blackhaul/pkg/version.Version=v1.2.3".
// "dev" means a local, untagged build.
var Version = "dev"
