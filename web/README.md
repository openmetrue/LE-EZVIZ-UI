# ezvizd (LE-EZVIZ-UI)

Web UI for EZVIZ cameras, built on **[LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS)**. This `web/` directory is a separate Go module so `go build ./...` at the repository root still builds only the original stream client. The daemon binary is still **`ezvizd`**.

[LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS) is the cloud stream client: it logs into EZVIZ, talks to VTM/VTDU, and writes the raw MPEG-PS feed. **`ezvizd`** turns that feed into low-latency WebRTC (H.264), with a site password, recordings, and a battery chart.

It is aimed at cameras with **no local RTSP** (HP2 and others). The camera sleeps until someone opens the page; the stream stops ~10s after the last viewer.

```
EZVIZ cloud (VTM/VTDU)
        │ MPEG-PS over TCP
        ▼
     le-ezviz-vs      (this repo root, fork of LE-EZVIZ-VS)
        │ FIFO (-out stream.ps, -idleWait)
        ▼
     ffmpeg           re-encode → Baseline H.264 (WebRTC) + raw ring for Save
        ▼
     ezvizd           auth, on-demand LivePub, archive, battery
        ▼
     browser          WebRTC (Safari / Chrome / iOS)
```

UI languages: English, Русский, 中文, Español, Deutsch, Français, 日本語, Português — switchable on the login page and in Settings.

This daemon is MIT. The stream client at the repo root stays LGPL-2.1 (upstream `LICENSE`).

## Layout in this repository

| Path | What |
|------|------|
| `/` (repo root) | Fork of [LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS), focused on MPEG-PS. Flags used by ezvizd: `-out`, `-idleWait`, `-maxStreamTime`, `-statusOnly`, env credentials. |
| `web/` | Sources for the **`ezvizd`** daemon — site password, WebRTC Live (`LivePub`), recordings (1 GB cap), battery history, logs, shareable token URL. |
| `deploy/` | Example `systemd` unit and nginx snippet. |

Upstream `protocol.md`, `codecs.md`, `encryption.md`, and `.github/workflows/go.yml` are unchanged. The root `readme.md` starts with the LE-EZVIZ-UI story; the rest is still LethalEthan’s original text.

## Build

Needs [Go](https://go.dev/dl/) 1.22+ and `ffmpeg` on the machine that runs the stream.

From the repository root:

```sh
make          # ezvizd + le-ezviz-vs for this OS
make linux    # amd64 binaries for a typical VPS
```

Do not commit the binaries, `config.json`, or EZVIZ credentials.

## Run the stream client locally

```sh
export EZVIZ_EMAIL='you@example.com' EZVIZ_PASSWORD='secret'
./le-ezviz-vs -region Russia -deviceSerial YOURSERIAL
# omit -deviceSerial to list devices
```

Idle-wait FIFO (what `ezvizd` uses): keep the EZVIZ session warm, start VTDU on a newline, stop it on `SIGUSR1`.

```sh
mkfifo stream.ps
./le-ezviz-vs -region Russia -deviceSerial YOURSERIAL -out stream.ps -idleWait -stdout=false -logFile=true -maxStreamTime 170
```

Stdout pipe still works for a one-shot dump: `-out=-` (without `-idleWait`).

## Deploy

From a GitHub Release (linux amd64):

```sh
curl -fsSL https://github.com/openmetrue/LE-EZVIZ-UI/releases/latest/download/install.sh | sudo bash
```

That is documented in the root `readme.md`. Manual copy (`make linux` → `/opt/ezvizd/`) still works.

Data lives in `/var/lib/ezvizd` (`recordings/`, `stats.json`, logs). Config is `/opt/ezvizd/config.json` (mode `600`, contains the EZVIZ password — keep it off git). Proxy `/ezviz/` to `127.0.0.1:8090` (see `deploy/nginx.conf`). Open the site once: set the **site** password, then EZVIZ email / password / serial.

## Battery / stream notes

- Cloud login (`-statusOnly`) does **not** wake the camera. Starting VTDU does.
- EZVIZ drops video for battery cams after ~180s even if keepalive is fine. The client reconnects at 170s (`-maxStreamTime`).
- Live is WebRTC H.264 (720p re-encode). Save remuxes the original camera MPEG-PS (full-res HEVC) from a short ring buffer.
- Client logs at Info. Debug logs every packet (~45 MB/day) and session URLs.

## Token URL

After setup, **Share** copies a token URL for Live (no site password). Treat it like a password.

## Patches on upstream LE-EZVIZ-VS

Applied on top of upstream files (not a wholesale replace):

- `main.go` — `-out` (stdout or FIFO), `-idleWait`, env credentials, `-statusOnly`, `-maxStreamTime`, reconnect loop; warnings on stderr so they do not corrupt the stream.
- `logging/log.go` — InfoLevel instead of Debug.
- `client/client.go` — `SetLogger`, `StreamOut`.
- `client/vtdu.go` — write MPEG-PS to `StreamOut`; `io.ReadFull`; reconnect on header desync.
- `client/pagelist.go` — `DeviceStatus`, `GetDeviceStatus` (original `GetPageList` unchanged).

## Lineage

| Piece | Name | Origin |
|------|------|--------|
| Stream client | `le-ezviz-vs` | [LethalEthan/LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS) |
| Web daemon | `ezvizd` | this fork (LE-EZVIZ-UI) |
