#!/usr/bin/env bash
# Build hikari from this checkout and install it in $HIKARI_PREFIX/bin
# (default /opt/hikari/bin), with that directory on the system PATH.
# It asks for sudo only on the first run, to make the directory and the PATH entry.
set -euo pipefail

prefix="${HIKARI_PREFIX:-/opt/hikari}"
bindir="$prefix/bin"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

make -C "$repo" build

if [ ! -d "$bindir" ] || [ ! -w "$bindir" ]; then
	echo "making $bindir (needs sudo once)"
	sudo mkdir -p "$bindir"
	sudo chown -R "$(id -un)" "$prefix"
fi

# The host binary looks for hikari-linux-<arch> next to itself, so they install together.
# Each file is copied beside its target and renamed over it, so a running hikari keeps its file.
for f in "$repo"/bin/hikari "$repo"/bin/hikari-linux-*; do
	name="$(basename "$f")"
	cp "$f" "$bindir/.$name.new"
	mv -f "$bindir/.$name.new" "$bindir/$name"
done

if [ -d /etc/paths.d ]; then
	entry=/etc/paths.d/hikari
	line="$bindir"
else
	entry=/etc/profile.d/hikari.sh
	line="export PATH=\"$bindir:\$PATH\""
fi
if [ "$(cat "$entry" 2>/dev/null)" != "$line" ]; then
	echo "adding $bindir to the PATH in $entry (needs sudo once)"
	echo "$line" | sudo tee "$entry" >/dev/null
fi

echo "installed $("$bindir/hikari" version) ($(git -C "$repo" rev-parse --short HEAD)) in $bindir"
case ":$PATH:" in
*":$bindir:"*) ;;
*) echo "open a new shell to put $bindir on your PATH" ;;
esac
