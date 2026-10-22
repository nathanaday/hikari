// Package version holds the version of this hikari build. The binary on the
// host and the Linux build that hikari mounts into each container come from
// one `make build`, so they share it.
package version

// Version is the semantic version of hikari.
const Version = "0.0.1"
