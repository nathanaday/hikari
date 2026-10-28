// Package version holds the version of this hikari build. The Makefile
// stamps the plugin's version into it, so the binary on the host, the Linux
// build that hikari mounts into each container, and the plugin share one.
package version

// Version is the semantic version of hikari, or dev for a build outside the
// Makefile.
var Version = "dev"
