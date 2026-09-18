# ezvizd (LE-EZVIZ-UI)

Web UI for EZVIZ cameras, built on **[LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS)**. This `web/` directory is a separate Go module so `go build ./...` at the repository root still builds only the original stream client. The daemon binary is still **`ezvizd`**.

[LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS) is the cloud stream client: it logs into EZVIZ, talks to VTM/VTDU, and writes the raw MPEG-PS feed. **`ezvizd`** turns that feed into low-latency HEVC over HTTP fMP4, with a site password, server-side clips (Videos tab), and a battery chart.

It is aimed at cameras with **no local RTSP** (HP2 and others). The camera sleeps until someone opens the page; the stream stops ~10s after the last viewer.

```
EZVIZ cloud (VTM/VTDU)
        │ MPEG-PS over TCP
        ▼
     le-ezviz-vs      (this repo root, fork of LE-EZVIZ-VS)
        │ FIFO (-out stream.ps, -idleWait)
        ▼
     ezvizd           Save ring + gomedia MPEG-PS demux
        │
        ├─ mp4ff CMAF fMP4 over HTTP /live — no ffmpeg transcode
        ▼
     Safari / Chrome (HEVC via MediaSource)
```

UI languages: English, Русский, 中文, Español, Deutsch, Français, 日本語, Português — switchable on the login page and in Settings.

This daemon is MIT. The stream client at the repo root stays LGPL-2.1 (upstream `LICENSE`).

## Layout in this repository

| Path | What |
|------|------|
| `/` (repo root) | Fork of [LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS), focused on MPEG-PS. Flags used by ezvizd: `-out`, `-idleWait`, `-maxStreamTime`, `-statusOnly`, env credentials. |
| `web/` | Sources for the **`ezvizd`** daemon — site password, clips on disk, battery history, logs. Live is gomedia MPEG-PS → HEVC → mp4ff CMAF fMP4 over HTTP, played with MediaSource. |
| `deploy/` | Example `systemd` unit and nginx snippet. |

Upstream `protocol.md`, `codecs.md`, `encryption.md`, and `.github/workflows/go.yml` are unchanged. The root `readme.md` starts with the LE-EZVIZ-UI story; the rest is still LethalEthan’s original text.

## Build

Needs [Go](https://go.dev/dl/) 1.24+ on the machine that runs the stream. No ffmpeg: Live and Save both demux the camera MPEG-PS with gomedia and mux with mp4ff / gomedia's MP4 muxer.

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
./le-ezviz-vs -region Russia -deviceSerial YOURSERIAL -out stream.ps -idleWait -maxStreamTime 170
```

Stdout pipe still works for a one-shot dump: `-out=-` (without `-idleWait`).

## Deploy

From a GitHub Release (linux amd64):

```sh
curl -fsSL https://github.com/openmetrue/LE-EZVIZ-UI/releases/latest/download/install.sh | sudo bash
```

That is documented in the root `readme.md`. The script adds nginx `/ezviz/` when nginx is present. Manual copy (`make linux` → `/opt/ezvizd/`) still works.

Data lives in `/var/lib/ezvizd` (`stats.json`, `devstatus.json`, `stream.ps` FIFO, `recordings/`, logs). Config is `/opt/ezvizd/config.json` (mode `600`, contains the EZVIZ password — keep it off git). Proxy `/ezviz/` to `127.0.0.1:8090` (see `deploy/nginx.conf`). Open the site once: set the **site** password, then EZVIZ email / password / serial.

## Battery / stream notes

- Cloud login (`-statusOnly`) does **not** wake the camera. Starting VTDU does.
- EZVIZ drops video for battery cams after ~180s even if keepalive is fine. The client reconnects at 170s (`-maxStreamTime`).
- Live is HTTP fMP4 HEVC (camera bitstream, no transcode), played with MediaSource. Save remuxes the original camera MPEG-PS (full-res HEVC) from a short ring buffer and stores it in `/var/lib/ezvizd/recordings/`; the Videos tab plays, downloads or deletes clips (oldest pruned past ~2 GB / 200 files). Safari 18+ / recent Chrome.
- Bridge logging is off by default. Enable it in Settings → Logs to write verbose logs (every packet, ~45 MB/day) to `lez.log`; it applies on the next stream start.

## Patches on upstream LE-EZVIZ-VS

Applied on top of upstream files (not a wholesale replace). Everything else in the
repository root is upstream, byte for byte — `logging/`, `client/request.go`, and the
cosmetic client files are untouched, so upstream edits to them merge cleanly.

- `main.go` — `-out` (stdout or FIFO), `-idleWait`, env credentials, `-statusOnly`, `-listDevices`, `-maxStreamTime`, reconnect loop; `-stdout`/`-logFile` pass through to the upstream logger (off by default); stdout logging is suppressed when stdout carries the stream or JSON, and `client.SetLogger` points the client at the configured logger so its logs cannot corrupt them.
- `client/client.go` — `SetLogger`, `StreamOut`, connection tracking (`TrackConn`/`InterruptStream`/`DropConns`), `dialTCP`.
- `client/vtdu.go` — write MPEG-PS to `StreamOut`; `io.ReadFull`; reconnect on header desync; no in-process ffmpeg remux.
- `client/vtm.go` — `DialVTM` (warm idle socket), simplified `ConnectVTM`.
- `client/pagelist.go` — `DeviceStatus`, `GetDeviceStatus` (original `GetPageList` unchanged).
- `client/serverinfo.go` — reject a response without `serverResp.authAddr` instead of dereferencing nil.
- `client/rtpPacket.go` — drop the unreachable tail (dead code; keeps `go vet` clean).
- `go.mod`/`go.sum` — no `ffmpeg-go`/`x/sync`.

`readme.md` and `.gitignore` also carry the fork's text on top of upstream.

Upstream sync is a merge: `git fetch upstream && git merge upstream/main`. Conflicts are
expected only in the files above plus `readme.md`. The logger is upstream's own, so
`ezvizd` turns bridge logging on/off by passing `-logFile` (Settings → Logs).

## Lineage

| Piece | Name | Origin |
|------|------|--------|
| Stream client | `le-ezviz-vs` | [LethalEthan/LE-EZVIZ-VS](https://github.com/LethalEthan/LE-EZVIZ-VS) |
| Web daemon | `ezvizd` | this fork (LE-EZVIZ-UI) |
