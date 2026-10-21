#!/bin/sh
# Create the demo shop repo at the given path, replacing what is there.
set -eu
dest=${1:?usage: make-repo.sh <path>}
src=$(cd "$(dirname "$0")/shop" && pwd)
rm -rf "$dest"
mkdir -p "$dest"
cp -R "$src/." "$dest/"
git -C "$dest" init --quiet --initial-branch=main
git -C "$dest" add --all
git -C "$dest" -c user.name="hikari demo" -c user.email=demo@hikari.invalid commit --quiet -m "Start the shop"
echo "created the demo repo at $dest"
