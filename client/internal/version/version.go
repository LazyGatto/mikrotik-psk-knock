// Package version carries the build version, shared by both binaries and the
// web UI. It follows semantic versioning (https://semver.org). Pre-1.0 (v0.x)
// the public surface — config schema, invite blob, RouterOS render — may still
// change between minor versions.
package version

// Version is the semantic version of this build. It defaults to "dev" for
// untagged local builds and is overridden at release time via:
//
//	go build -ldflags "-X mikrotik-psk-knock/client/internal/version.Version=v0.1.0"
var Version = "dev"

// String returns the version, guaranteeing a non-empty value.
func String() string {
	if Version == "" {
		return "dev"
	}
	return Version
}
