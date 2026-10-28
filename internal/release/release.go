// Package release pins the binaries that the plugin installs. A release
// builds hikari for each platform the same way on every machine, so the
// sha256 of each file is known before the tag; the plugin carries those
// checksums, and its launcher installs a download only when it matches them.
package release

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed launcher.sh
var launcherTemplate string

// Platforms are the os/arch pairs each release builds. The Linux binaries
// are also the ones that hikari mounts into its containers.
var Platforms = []string{"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"}

// Asset is the file name of the binary of a version for one platform.
func Asset(version, platform string) string {
	return "hikari-" + version + "-" + strings.Replace(platform, "/", "-", 1)
}

// ChecksumsFile holds the sha256 of each asset of the plugin's version, as
// sha256sum prints them.
const ChecksumsFile = "plugin/release/checksums.txt"

// LauncherFile is the launcher that the plugin puts on the PATH.
const LauncherFile = "plugin/bin/hikari"

// Launcher is the launcher for a version and its checksums.
func Launcher(version, checksums string) string {
	r := strings.NewReplacer("@VERSION@", version, "@CHECKSUMS@", strings.TrimSpace(checksums))
	return r.Replace(launcherTemplate)
}

// ParseChecksums reads a checksums file into asset → sha256, and refuses a
// malformed line.
func ParseChecksums(text string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 || len(f[0]) != 64 {
			return nil, fmt.Errorf("%s: %q is not <sha256>  <file>", ChecksumsFile, line)
		}
		out[f[1]] = f[0]
	}
	return out, nil
}

// Missing lists the assets of a version that the checksums do not name.
func Missing(version string, sums map[string]string) []string {
	var out []string
	for _, p := range Platforms {
		if _, ok := sums[Asset(version, p)]; !ok {
			out = append(out, Asset(version, p))
		}
	}
	sort.Strings(out)
	return out
}
