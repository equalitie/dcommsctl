#!/usr/bin/env bash
# Builds dist/dcommsctl-<version>-linux-<arch>.tar.gz (+ .sha256) for the dComms App installer.
# Usage: ./release.sh [version]   (default: first line of .dcomms-installation)
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$PWD"

VERSION="${1:-}"
if [ -z "$VERSION" ] && [ -f .dcomms-installation ]; then
  VERSION="$(head -n1 .dcomms-installation | tr -d '[:space:]')"
fi
[ -n "$VERSION" ] || { echo "usage: $0 <version>  (or put it on the first line of .dcomms-installation)" >&2; exit 1; }

ARCHES="${ARCHES:-amd64 arm64}"
DIST="$ROOT/dist"
EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct 2>/dev/null || echo 0)}"

die() { echo "release: $*" >&2; exit 1; }

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{print $1}'
}

TAR=tar
command -v gtar >/dev/null && TAR=gtar
if "$TAR" --version 2>/dev/null | grep -q 'GNU tar'; then
  TAR_FLAGS=(--format=ustar --owner=0 --group=0 --numeric-owner --no-recursion)
else
  TAR_FLAGS=(--format=ustar --uid 0 --gid 0 --uname root --gname root -n --no-xattrs --no-acls --no-mac-metadata)
fi
export COPYFILE_DISABLE=1

for dir in conf_tmpl/caddy conf_tmpl/dawa; do [ -d "$dir" ] || die "missing $dir"; done
[ -x scripts/docker_setup.sh ] || die "missing or non-executable scripts/docker_setup.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$DIST"

for arch in $ARCHES; do
  name="dcommsctl-$VERSION-linux-$arch"
  tarball="$name.tar.gz"
  stage="$WORK/$name"
  echo "==> $tarball"

  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -buildid=" -o "$stage/dcommsctl" ./cmd

  cp -R conf_tmpl scripts "$stage/"
  find "$stage" \( -name '.DS_Store' -o -name '._*' \) -delete

  # Normalise modes and mtimes so the same source gives the same bytes.
  find "$stage" -type d -exec chmod 0755 {} +
  find "$stage" -type f -exec chmod 0644 {} +
  find "$stage" -type f \( -name '*.sh' -o -name dcommsctl \) -exec chmod 0755 {} +
  stamp="$(TZ=UTC date -r "$EPOCH" +%Y%m%d%H%M.%S 2>/dev/null || TZ=UTC date -d "@$EPOCH" +%Y%m%d%H%M.%S)"
  find "$stage" -exec env TZ=UTC touch -h -t "$stamp" {} +

  (cd "$stage" && find . -mindepth 1 | LC_ALL=C sort) > "$WORK/files"
  "$TAR" -C "$stage" "${TAR_FLAGS[@]}" -cf - -T "$WORK/files" | gzip -9n > "$DIST/$tarball"
  (cd "$DIST" && printf '%s  %s\n' "$(sha256 "$tarball")" "$tarball" > "$tarball.sha256")

  # Repeat the installer's own verify and unpack checks.
  check="$WORK/check-$arch"
  mkdir -p "$check"
  [ "$(sha256 "$DIST/$tarball")" = "$(awk '{print $1}' "$DIST/$tarball.sha256")" ] || die "$tarball: checksum mismatch"
  tar -xzf "$DIST/$tarball" -C "$check" || die "$tarball: could not be unpacked"
  [ -x "$check/dcommsctl" ] || die "$tarball: no executable dcommsctl"
  [ -x "$check/scripts/docker_setup.sh" ] || die "$tarball: no executable scripts/docker_setup.sh"
  for app in caddy dawa; do [ -d "$check/conf_tmpl/$app" ] || die "$tarball: no conf_tmpl/$app"; done
  if command -v file >/dev/null; then
    file "$check/dcommsctl" | grep -q 'ELF 64-bit' || die "$tarball: dcommsctl is not a linux binary"
    file "$check/dcommsctl" | grep -Eq 'statically linked|static-pie' || die "$tarball: dcommsctl is not static"
  fi

  echo "    $(cat "$DIST/$tarball.sha256")"
done

echo "done: upload everything in $DIST to the URL in kDcommsctlDownloadUrl"
