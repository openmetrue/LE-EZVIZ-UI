#!/usr/bin/env bash
# One-shot install / upgrade of LE-EZVIZ-UI (ezvizd + le-ezviz-vs).
#   curl -fsSL https://github.com/openmetrue/LE-EZVIZ-UI/releases/latest/download/install.sh | sudo bash
set -euo pipefail

REPO="${EZVIZ_REPO:-openmetrue/LE-EZVIZ-UI}"
VERSION="${EZVIZ_VERSION:-latest}"
PREFIX="${EZVIZ_PREFIX:-/opt/ezvizd}"
WORKDIR="${EZVIZ_WORKDIR:-/var/lib/ezvizd}"
BASE="${EZVIZ_BASE:-/ezviz}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root (sudo)" >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  *)
    echo "unsupported architecture: $arch (linux amd64 only for now)" >&2
    exit 1
    ;;
esac

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "this installer targets Linux + systemd" >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemd is required" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v ffmpeg >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq ffmpeg >/dev/null
  else
    echo "install ffmpeg and re-run" >&2
    exit 1
  fi
fi

if [[ "$VERSION" == "latest" ]]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fetch() {
  local name="$1"
  echo "downloading ${name}"
  curl -fsSL "${base_url}/${name}" -o "${tmp}/${name}"
}

fetch "ezvizd-linux-${arch}"
fetch "le-ezviz-vs-linux-${arch}"
fetch "ezvizd.service"
if curl -fsSL "${base_url}/sha256sums.txt" -o "${tmp}/sha256sums.txt"; then
  (
    cd "$tmp"
    sha256sum -c --ignore-missing sha256sums.txt
  )
else
  echo "warning: no sha256sums.txt in this release, skipping checksums" >&2
fi

if systemctl is-active --quiet ezvizd 2>/dev/null; then
  systemctl stop ezvizd
fi

mkdir -p "$PREFIX" "$WORKDIR"
install -m 0755 "${tmp}/ezvizd-linux-${arch}" "${PREFIX}/ezvizd"
install -m 0755 "${tmp}/le-ezviz-vs-linux-${arch}" "${PREFIX}/le-ezviz-vs"
# Keep operator edits to ExecStart; only install the unit when missing.
if [[ ! -f /etc/systemd/system/ezvizd.service ]]; then
  install -m 0644 "${tmp}/ezvizd.service" /etc/systemd/system/ezvizd.service
fi

systemctl daemon-reload
systemctl enable --now ezvizd

echo
echo "ezvizd is running on 127.0.0.1:8090 (URL prefix ${BASE}/)."
echo "Point your TLS reverse proxy at it, for example:"
echo
echo "  location ${BASE}/ {"
echo "      proxy_pass         http://127.0.0.1:8090;"
echo "      proxy_http_version 1.1;"
echo "      proxy_buffering    off;"
echo "      proxy_read_timeout 3600s;"
echo "      proxy_set_header   Host \$host;"
echo "      proxy_set_header   X-Real-IP \$remote_addr;"
echo "      proxy_set_header   X-Forwarded-Proto \$scheme;"
echo "  }"
echo
echo "Then open https://your.domain${BASE}/ and set the site password."
echo "Existing ${PREFIX}/config.json is left untouched."
