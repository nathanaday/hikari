#!/usr/bin/env bash
# Build hikari from this checkout and install it where the plugin's launcher
# runs it: $HIKARI_HOME/bin/<version>/ (default ~/.hikari), with
# $HIKARI_HOME/bin/hikari pointing at it, and $HIKARI_HOME/bin on the system
# PATH. It asks for sudo only on the first run, to write the PATH entry.
set -euo pipefail

home="${HIKARI_HOME:-$HOME/.hikari}"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

make -C "$repo" build
version="$("$repo/bin/hikari" version | awk '{ print $2 }')"
dir="$home/bin/$version"
mkdir -p "$dir"

# The host binary looks for hikari-linux-<arch> next to itself, so they install together.
# Each file is copied beside its target and renamed over it, so a running hikari keeps its file.
for f in "$repo"/bin/hikari "$repo"/bin/hikari-linux-*; do
	name="$(basename "$f")"
	cp "$f" "$dir/.$name.new"
	mv -f "$dir/.$name.new" "$dir/$name"
done
ln -s "$version/hikari" "$home/bin/.link.$$"
mv -f "$home/bin/.link.$$" "$home/bin/hikari"

bindir="$home/bin"
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

echo "installed $("$dir/hikari" version) ($(git -C "$repo" rev-parse --short HEAD)) in $dir"
if [ -d /opt/hikari/bin ]; then
	echo "an older install is in /opt/hikari; remove it with: sudo rm -rf /opt/hikari"
fi
case ":$PATH:" in
*":$bindir:"*) ;;
*) echo "open a new shell to put $bindir on your PATH" ;;
esac
