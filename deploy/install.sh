#!/usr/bin/env bash
# One-shot install / upgrade of LE-EZVIZ-UI (ezvizd + le-ezviz-vs).
#   curl -fsSL https://github.com/openmetrue/LE-EZVIZ-UI/releases/latest/download/install.sh | sudo bash
# Pin: EZVIZ_VERSION=vX.Y.Z bash
# Matching versions still repair runtime (ffmpeg libs, nginx).
set -euo pipefail

REPO="${EZVIZ_REPO:-openmetrue/LE-EZVIZ-UI}"
VERSION="${EZVIZ_VERSION:-latest}"
PREFIX="${EZVIZ_PREFIX:-/opt/ezvizd}"
WORKDIR="${EZVIZ_WORKDIR:-/var/lib/ezvizd}"
BASE="${EZVIZ_BASE:-/ezviz}"
UNIT=/etc/systemd/system/ezvizd.service

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

apt_install() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -o DPkg::Lock::Timeout=120 \
    ca-certificates curl ffmpeg libblas3 liblapack3
}

ffmpeg_ok() {
  command -v ffmpeg >/dev/null 2>&1 || return 1
  ffmpeg -hide_banner -version >/dev/null 2>&1
}

ensure_runtime() {
  local need_apt=0
  if ! command -v curl >/dev/null 2>&1; then
    need_apt=1
  fi
  if ! ffmpeg_ok; then
    need_apt=1
  fi
  if [[ "$need_apt" -eq 1 ]]; then
    if ! command -v apt-get >/dev/null 2>&1; then
      echo "install curl, ffmpeg, libblas3, liblapack3 and re-run" >&2
      exit 1
    fi
    echo "installing runtime packages (curl, ffmpeg, libblas3, liblapack3)"
    apt_install
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required" >&2
    exit 1
  fi
  if ! ffmpeg_ok; then
    echo "ffmpeg is missing or cannot start (shared libraries?)" >&2
    exit 1
  fi
  if [[ ! -x /usr/bin/ffmpeg ]]; then
    echo "ezvizd.service expects /usr/bin/ffmpeg" >&2
    exit 1
  fi
}

# Older releases shipped go2rtc. Live is a plain HTTP fMP4 stream now; drop the
# flag, binary, and logs.
cleanup_go2rtc() {
  rm -f "${PREFIX}/go2rtc" "${WORKDIR}/go2rtc.yaml" "${WORKDIR}/go2rtc.log" "${WORKDIR}/live.ps"
  [[ -f "$UNIT" ]] || return 1
  grep -q -- '-go2rtc' "$UNIT" || return 1
  local tmp
  tmp="$(mktemp)"
  awk '
    /^[[:space:]]*#/ { print; next }
    /^[[:space:]]*-go2rtc[[:space:]]/ { next }
    {
      gsub(/[[:space:]]+-go2rtc[[:space:]]+[^[:space:]\\]+/, "")
      print
    }
  ' "$UNIT" >"$tmp"
  if cmp -s "$tmp" "$UNIT"; then
    rm -f "$tmp"
    return 1
  fi
  install -m 0644 "$tmp" "$UNIT"
  rm -f "$tmp"
  echo "removed leftover -go2rtc from ${UNIT}"
}

write_nginx_snippet() {
  mkdir -p /etc/nginx/snippets
  cat > /etc/nginx/snippets/ezvizd.conf <<EOF
location ${BASE}/ {
    proxy_pass         http://127.0.0.1:8090;
    proxy_http_version 1.1;
    proxy_buffering    off;
    proxy_read_timeout 3600s;
    proxy_set_header   Host \$host;
    proxy_set_header   X-Real-IP \$remote_addr;
    proxy_set_header   X-Forwarded-Proto \$scheme;
    proxy_set_header   X-Forwarded-Host \$host;
}
EOF
}

nginx_has_ezviz() {
  grep -Rql -E "location[[:space:]]+${BASE}/|snippets/ezvizd\\.conf" \
    /etc/nginx/sites-enabled /etc/nginx/sites-available /etc/nginx/conf.d \
    2>/dev/null
}

ensure_nginx() {
  if ! command -v nginx >/dev/null 2>&1; then
    return 0
  fi
  write_nginx_snippet
  if nginx_has_ezviz; then
    echo "nginx already proxies ${BASE}/"
    return 0
  fi
  local site=""
  if [[ -f /etc/nginx/sites-available/default ]]; then
    site=/etc/nginx/sites-available/default
  fi
  if [[ -z "$site" ]]; then
    echo "Add this inside your TLS server block:  include /etc/nginx/snippets/ezvizd.conf;"
    return 0
  fi
  local bak="${site}.bak-ezvizd"
  cp -a "$site" "$bak"
  if ! awk '
    { lines[NR] = $0 }
    END {
      last = 0
      for (i = 1; i <= NR; i++) {
        if (lines[i] ~ /^[[:space:]]*}[[:space:]]*$/) last = i
      }
      if (last == 0) exit 1
      for (i = 1; i <= NR; i++) {
        if (i == last) print "    include /etc/nginx/snippets/ezvizd.conf;"
        print lines[i]
      }
    }
  ' "$bak" >"$site"; then
    mv "$bak" "$site"
    echo "warning: could not patch ${site}; include snippets/ezvizd.conf manually" >&2
    return 0
  fi
  if nginx -t >/dev/null 2>&1; then
    rm -f "$bak"
    systemctl reload nginx 2>/dev/null || true
    echo "nginx: added ${BASE}/ to ${site}"
  else
    mv "$bak" "$site"
    echo "warning: nginx -t failed after patch; left ${site} unchanged" >&2
    echo "Add this inside your TLS server block:  include /etc/nginx/snippets/ezvizd.conf;"
  fi
}

print_done() {
  local ver="$1"
  echo
  echo "installed ${ver}"
  echo "ezvizd is running on 127.0.0.1:8090 (URL prefix ${BASE}/)."
  if ! command -v nginx >/dev/null 2>&1; then
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
    echo "      proxy_set_header   X-Forwarded-Host \$host;"
    echo "  }"
    echo
  fi
  echo "Then open https://your.domain${BASE}/ and set the site password."
  echo "Existing ${PREFIX}/config.json is left untouched."
}

ensure_runtime

norm_ver() {
  local v="${1:-}"
  v="${v#v}"
  printf '%s' "$v"
}

resolve_version() {
  if [[ "$VERSION" != "latest" ]]; then
    printf '%s' "$VERSION"
    return
  fi
  local tag
  # Prefer github.com assets: api.github.com often 403s unauthenticated VPS IPs.
  tag="$(curl -fsSL -H 'User-Agent: ezvizd-install' \
    "https://github.com/${REPO}/releases/latest/download/VERSION" | tr -d '[:space:]' || true)"
  if [[ -z "$tag" ]]; then
    tag="$(curl -fsSL -o /dev/null -w '%{redirect_url}' -H 'User-Agent: ezvizd-install' \
      "https://github.com/${REPO}/releases/latest" || true)"
    tag="${tag##*/releases/tag/}"
    tag="${tag%%[/?]*}"
  fi
  if [[ -z "$tag" ]]; then
    echo "could not resolve latest release tag for ${REPO}" >&2
    echo "set EZVIZ_VERSION=vX.Y.Z and download that tag's install.sh instead of /latest/" >&2
    exit 1
  fi
  printf '%s' "$tag"
}

TARGET="$(resolve_version)"
CURRENT=""
if [[ -f "${PREFIX}/VERSION" ]]; then
  CURRENT="$(tr -d '[:space:]' <"${PREFIX}/VERSION")"
elif [[ -x "${PREFIX}/ezvizd" ]]; then
  CURRENT="$("${PREFIX}/ezvizd" -version 2>/dev/null || true)"
fi

skip_binaries=0
if [[ -n "$CURRENT" && "$(norm_ver "$CURRENT")" == "$(norm_ver "$TARGET")" && -f "$UNIT" ]]; then
  skip_binaries=1
fi

if [[ "$skip_binaries" -eq 0 ]]; then
  base_url="https://github.com/${REPO}/releases/download/${TARGET}"

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  fetch() {
    local name="$1"
    echo "downloading ${name} (${TARGET})"
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

  echo "download complete — installing ${TARGET}"
  if [[ -n "${EZVIZ_UPDATE_STATUS:-}" ]]; then
    printf '{"phase":"install","target":"%s","error":"","at":"%s"}\n' \
      "$TARGET" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$EZVIZ_UPDATE_STATUS"
  fi
  was_active=0
  if systemctl is-active --quiet ezvizd 2>/dev/null; then
    was_active=1
    systemctl stop ezvizd
  fi
  restore_on_fail() {
    if [[ "$was_active" -eq 1 ]]; then
      systemctl start ezvizd 2>/dev/null || true
    fi
  }
  trap restore_on_fail ERR

  mkdir -p "$PREFIX" "$WORKDIR"
  install -m 0755 "${tmp}/ezvizd-linux-${arch}" "${PREFIX}/ezvizd"
  install -m 0755 "${tmp}/le-ezviz-vs-linux-${arch}" "${PREFIX}/le-ezviz-vs"
  printf '%s\n' "$TARGET" >"${PREFIX}/VERSION"
  # Keep operator edits to ExecStart; only install the unit when missing.
  if [[ ! -f "$UNIT" ]]; then
    install -m 0644 "${tmp}/ezvizd.service" "$UNIT"
  fi
  trap - ERR
fi

unit_changed=0
if cleanup_go2rtc; then
  unit_changed=1
fi

ensure_nginx

systemctl daemon-reload
systemctl enable ezvizd >/dev/null
if [[ "$skip_binaries" -eq 1 && "$unit_changed" -eq 1 ]] && systemctl is-active --quiet ezvizd; then
  systemctl restart ezvizd
else
  systemctl start ezvizd
fi

if [[ "$skip_binaries" -eq 1 ]]; then
  echo "already at ${CURRENT} — binaries unchanged, runtime checked"
fi
print_done "$TARGET"
