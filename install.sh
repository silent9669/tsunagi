#!/bin/sh
# Installs the latest tsunagi release for macOS into /usr/local/bin (or $TSUNAGI_BIN).
set -eu

repo="silent9669/tsunagi"
bin_dir="${TSUNAGI_BIN:-/usr/local/bin}"

[ "$(uname -s)" = "Darwin" ] || { echo "tsunagi supports macOS only" >&2; exit 1; }
case "$(uname -m)" in
  arm64) arch=arm64 ;;
  x86_64) arch=amd64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tag="${TSUNAGI_VERSION:-$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)}"
[ -n "$tag" ] || { echo "could not find the latest release" >&2; exit 1; }
version="${tag#v}"
name="tsunagi_${version}_darwin_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$tag"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$base/$name" -o "$tmp/$name"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
want="$(grep " $name\$" "$tmp/checksums.txt" | awk '{print $1}')"
got="$(shasum -a 256 "$tmp/$name" | awk '{print $1}')"
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "checksum mismatch for $name" >&2; exit 1; }

tar -xzf "$tmp/$name" -C "$tmp" tsunagi
if [ -w "$bin_dir" ]; then
  install -m 0755 "$tmp/tsunagi" "$bin_dir/tsunagi"
else
  sudo install -m 0755 "$tmp/tsunagi" "$bin_dir/tsunagi"
fi
echo "installed tsunagi $version to $bin_dir/tsunagi"
