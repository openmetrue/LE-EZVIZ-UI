// LivePlayer — HTTP CMAF fMP4 into MediaSource. duration is Infinity (live).
// GET /live starts immediately so the first IDR is not missed. A new ftyp
// rebuilds the SourceBuffer on the same HTTP stream (pipeline restart).
(function (global) {
  "use strict";

  const HEVC_TYPES = [
    'video/mp4; codecs="hvc1.1.6.L120.B0"',
    'video/mp4; codecs="hvc1.1.6.L93.B0"',
    'video/mp4; codecs="hev1.1.6.L120.B0"',
    'video/mp4; codecs="hvc1"',
  ];

  // Live-edge control. A reload is handed the whole in-progress GOP, so playback
  // starts behind the newest frame; these keep pulling it back to the edge.
  const LIVE_SEEK_TARGET = 0.5;   // start this far behind the buffered end
  const LIVE_SOFT_AHEAD = 1.0;    // speed up slightly past this much buffer
  const LIVE_HARD_AHEAD = 2.5;    // jump forward past this much buffer
  const LIVE_MAX_RATE = 1.15;
  const LIVE_ATTACH_WAIT = 1500;  // start even if the edge marker never arrives

  function qs(token) {
    return token ? ("?token=" + encodeURIComponent(token)) : "";
  }

  // A hung server must not stall the tick/schedule loop: abort after ms.
  function fetchWithTimeout(url, opts, ms) {
    const ac = new AbortController();
    const timer = setTimeout(function () { ac.abort(); }, ms);
    const merged = Object.assign({}, opts || {}, { signal: ac.signal });
    return fetch(url, merged).finally(function () { clearTimeout(timer); });
  }

  function mediaSourceCtor() {
    return global.ManagedMediaSource || global.MediaSource || null;
  }

  function mimeFromCodec(codec) {
    if (!codec) return "";
    return 'video/mp4; codecs="' + codec + '"';
  }

  function pickMime(MS, codec) {
    if (!MS || typeof MS.isTypeSupported !== "function") return "";
    const listed = [];
    const fromStatus = mimeFromCodec(codec);
    if (fromStatus) listed.push(fromStatus);
    for (let i = 0; i < HEVC_TYPES.length; i++) listed.push(HEVC_TYPES[i]);
    for (let i = 0; i < listed.length; i++) {
      try {
        if (MS.isTypeSupported(listed[i])) return listed[i];
      } catch (_) {}
    }
    return "";
  }

  function isUserStatus(text, strings) {
    if (!text) return false;
    const s = strings || {};
    return (
      text === s.saving ||
      (s.downloaded && text.indexOf(s.downloaded) === 0) ||
      (s.saveFail && text.indexOf(s.saveFail) === 0)
    );
  }

  function boxType(u8) {
    if (!u8 || u8.length < 8) return "";
    return String.fromCharCode(u8[4], u8[5], u8[6], u8[7]);
  }

  function isInitSeg(u8) {
    return boxType(u8) === "ftyp";
  }

  // Sent by the server right after the pre-attach GOP. Four bytes "EDGE", so it
  // never collides with an fMP4 box (those are at least 8 bytes).
  function isLiveEdge(u8) {
    return u8 && u8.length === 4 && u8[0] === 0x45 && u8[1] === 0x44 && u8[2] === 0x47 && u8[3] === 0x45;
  }

  function LivePlayer() {
    this.base = "";
    this.token = "";
    this.strings = {};
    this.gen = 0;
    this.busy = false;
    this.running = false;
    this.playing = false;
    this.liveOn = false;
    this.lastSerial = "";
    this.codec = "";
    this.loopTimer = 0;
    this.needFresh = false;
    this.abort = null;
    this.objectURL = "";
  }

  LivePlayer.prototype.start = function (opts) {
    opts = opts || {};
    this.base = opts.base || "";
    this.token = opts.token || "";
    this.strings = opts.strings || {};
    this.running = true;
    const self = this;
    document.addEventListener("visibilitychange", function () {
      const v = document.getElementById("v");
      if (document.hidden) {
        if (v) v.pause();
        return;
      }
      if (v && (v.src || v.srcObject)) v.play().catch(function () {});
      self.tick();
    });
    this.tick();
    this.schedule();
    return this;
  };

  LivePlayer.prototype.stop = function () {
    this.running = false;
    this.gen++;
    if (this.loopTimer) clearTimeout(this.loopTimer);
    this.closeLive();
  };

  LivePlayer.prototype.notifyDeviceSwitch = function () {
    this.gen++;
    this.playing = false;
    this.needFresh = true;
    this.closeLive();
    this.setStatus(this.strings.waiting || "Waiting for camera…");
  };

  LivePlayer.prototype.schedule = function () {
    const self = this;
    if (!this.running) return;
    const delay = this.playing ? 2000 : 150;
    this.loopTimer = setTimeout(function () {
      self.tick().finally(function () {
        self.schedule();
      });
    }, delay);
  };

  LivePlayer.prototype.url = function (path) {
    return this.base + path + qs(this.token);
  };

  LivePlayer.prototype.setStatus = function (text) {
    const el = document.getElementById("st");
    if (!el) return;
    if (isUserStatus(el.textContent, this.strings) && !text) return;
    el.textContent = text || "";
  };

  LivePlayer.prototype.setBattery = function (device) {
    const el = document.getElementById("bat");
    if (!el) return;
    const d = device || {};
    if (d.battery === undefined || d.battery === null || d.battery === "") {
      el.textContent = "";
      return;
    }
    const s = this.strings;
    let t = (s.battery || "Battery") + ": " + d.battery + "%";
    if (d.wifi_signal) t += " · Wi-Fi: " + d.wifi_signal + "%";
    t += " · " + (d.online ? (s.online || "online") : (s.offline || "offline"));
    if (d.upgrade_available === 1) t += " · " + (s.upgrade || "firmware update available");
    el.textContent = t;
  };

  LivePlayer.prototype.setMode = function (st) {
    const el = document.getElementById("mode");
    if (!el) return;
    el.textContent = st.stream_mode === "always" ? (this.strings.alwaysOn || "") : "";
  };

  LivePlayer.prototype.closeLive = function () {
    this.busy = false;
    this.playing = false;
    this.liveOn = false;
    if (this.abort) {
      try { this.abort.abort(); } catch (_) {}
      this.abort = null;
    }
    if (this.objectURL) {
      try { URL.revokeObjectURL(this.objectURL); } catch (_) {}
      this.objectURL = "";
    }
  };

  LivePlayer.prototype.armVideo = function (v) {
    if (!v) return;
    v.muted = true;
    v.defaultMuted = true;
    v.volume = 0;
    v.autoplay = true;
    v.playsInline = true;
    v.setAttribute("playsinline", "");
    v.setAttribute("webkit-playsinline", "");
    try { v.disableRemotePlayback = true; } catch (_) {}
    const self = this;
    if (!v._liveKeep) {
      v._liveKeep = true;
      ["waiting", "stalled", "suspend"].forEach(function (ev) {
        v.addEventListener(ev, function () {
          if (!self.running || document.hidden || (!v.src && !v.srcObject)) return;
          v.play().catch(function () {});
        });
      });
    }
  };

  LivePlayer.prototype.tick = async function () {
    if (!this.running || document.hidden) return;
    const fresh = this.needFresh;
    let st;
    try {
      const statusURL = this.url("/api/status") + (fresh ? (this.token ? "&fresh=1" : "?fresh=1") : "");
      const r = await fetchWithTimeout(statusURL, { cache: "no-store" }, 5000);
      if (r.status === 401) {
        this.setStatus(this.strings.relogin || "Please sign in again.");
        return;
      }
      st = await r.json();
      if (fresh) this.needFresh = false;
    } catch (_) {
      this.setStatus(this.strings.reconnecting || "Reconnecting…");
      return;
    }
    this.setBattery(st.device);
    this.setMode(st);
    if (st.codec) this.codec = st.codec;

    if (!st.configured) {
      this.setStatus(this.strings.notConfigured || "Camera is not set up — open Settings.");
      return;
    }

    if (st.active_serial && this.lastSerial && st.active_serial !== this.lastSerial && this.liveOn) {
      this.notifyDeviceSwitch();
    }
    if (st.active_serial) this.lastSerial = st.active_serial;

    if (!this.liveOn && !this.busy) {
      this.openLive();
    }
    if (!this.playing) {
      if (st.starting || st.running) {
        this.setStatus(this.strings.waiting || "Waiting for camera…");
      } else {
        const err = st.last_error ? ((this.strings.lastError || "Last error: ") + st.last_error) : "";
        this.setStatus(err || this.strings.waiting || "Waiting for camera…");
      }
    }
  };

  LivePlayer.prototype.openLive = async function () {
    if (this.busy || !this.running) return;
    const MS = mediaSourceCtor();
    const mime = pickMime(MS, this.codec);
    if (!MS || !mime) {
      this.setStatus(this.strings.noRtc || "This browser cannot play HEVC video.");
      return;
    }
    const gen = ++this.gen;
    this.busy = true;
    this.liveOn = true;
    const ac = new AbortController();
    this.abort = ac;
    const v = document.getElementById("v");
    this.armVideo(v);
    const liveFetch = fetch(this.url("/live"), { signal: ac.signal, cache: "no-store" });
    const self = this;

    try {
      const r = await liveFetch;
      if (gen !== self.gen) return;
      if (r.status === 401) {
        self.setStatus(self.strings.relogin || "Please sign in again.");
        return;
      }
      if (!r.ok || !r.body) {
        self.setStatus(self.strings.reconnecting || "Reconnecting…");
        self.busy = false;
        self.liveOn = false;
        return;
      }
      const stream = { reader: r.body.getReader(), buf: new Uint8Array(0), off: 0 };
      let leftover = null;
      while (gen === self.gen) {
        leftover = await attachMSE(MS, mime, stream, v, self, gen, leftover);
        if (!leftover) break;
      }
      if (gen === self.gen) {
        self.busy = false;
        self.liveOn = false;
        self.playing = false;
      }
    } catch (_) {
      if (gen !== self.gen) return;
      self.busy = false;
      self.liveOn = false;
      self.playing = false;
    }
  };

  async function attachMSE(MS, mime, stream, v, self, gen, first) {
    const ms = new MS();
    if (self.objectURL) {
      try { URL.revokeObjectURL(self.objectURL); } catch (_) {}
    }
    self.objectURL = URL.createObjectURL(ms);
    v.srcObject = null;
    v.src = self.objectURL;
    await new Promise(function (resolve, reject) {
      ms.addEventListener("sourceopen", function () { resolve(); }, { once: true });
      ms.addEventListener("error", function () { reject(new Error("mse")); }, { once: true });
    });
    if (gen !== self.gen) return null;
    try { ms.duration = Infinity; } catch (_) {}
    const sb = ms.addSourceBuffer(mime);
    sb.mode = "sequence";
    return pumpLive(stream, sb, v, self, gen, first);
  }

  function compactStream(stream) {
    if (stream.off === 0) return;
    stream.buf = stream.buf.subarray(stream.off);
    stream.off = 0;
  }

  function nextFragFromBuf(stream) {
    compactStream(stream);
    if (stream.buf.length - stream.off < 4) return null;
    const o = stream.off;
    const n = ((stream.buf[o] << 24) | (stream.buf[o + 1] << 16) | (stream.buf[o + 2] << 8) | stream.buf[o + 3]) >>> 0;
    if (n <= 0 || stream.buf.length - o < 4 + n) return null;
    const frag = stream.buf.subarray(o + 4, o + 4 + n).slice();
    stream.off = o + 4 + n;
    return frag;
  }

  async function readFrag(stream) {
    while (true) {
      const frag = nextFragFromBuf(stream);
      if (frag) return frag;
      const next = await stream.reader.read();
      if (next.done) return null;
      compactStream(stream);
      const chunk = next.value;
      const merged = new Uint8Array(stream.buf.length + chunk.length);
      merged.set(stream.buf, 0);
      merged.set(chunk, stream.buf.length);
      stream.buf = merged;
    }
  }

  async function pumpLive(stream, sb, v, self, gen, first) {
    const queue = [];
    let lastPrune = 0;
    let sawMedia = false;
    let backlogComplete = false;
    let liveStarted = false;
    let attachTimer = 0;
    let governorTimer = 0;

    function bufferedRange() {
      if (!v.buffered || !v.buffered.length) return null;
      return { start: v.buffered.start(0), end: v.buffered.end(v.buffered.length - 1) };
    }

    function flush() {
      if (sb.updating || !queue.length) return;
      try {
        sb.appendBuffer(queue[0]);
        queue.shift();
      } catch (e) {
        const quota = e && (e.name === "QuotaExceededError" || e.code === 22);
        if (quota) {
          try {
            if (v.buffered && v.buffered.length) {
              const t = v.currentTime || v.buffered.end(0);
              const s0 = v.buffered.start(0);
              if (t - s0 > 15) sb.remove(s0, t - 12);
            }
          } catch (_) {}
        }
      }
    }

    function prune() {
      const now = Date.now();
      if (now - lastPrune < 8000 || !self.playing || sb.updating) return;
      if (!v.buffered || !v.buffered.length) return;
      const t = v.currentTime;
      if (!(t > 12)) return;
      try {
        const start = v.buffered.start(0);
        if (t - start > 20) {
          sb.remove(start, t - 15);
          lastPrune = now;
        }
      } catch (_) {}
    }

    function startPlayback() {
      if (self.playing || gen !== self.gen) return;
      v.play().then(function () {
        self.playing = true;
        self.setStatus("");
      }).catch(function () {});
    }

    function releaseTimers() {
      if (attachTimer) { clearTimeout(attachTimer); attachTimer = 0; }
      if (governorTimer) { clearInterval(governorTimer); governorTimer = 0; }
    }

    function setRate(r) {
      if (v.playbackRate === r) return;
      try { v.playbackRate = r; } catch (_) {}
    }

    function seekToEdge() {
      const b = bufferedRange();
      if (!b) return;
      const target = Math.max(b.start, b.end - LIVE_SEEK_TARGET);
      if (target - (v.currentTime || 0) < 0.05) return;
      try { v.currentTime = target; } catch (_) {}
    }

    // Keep playback pinned to the newest buffered frame. Without this a stall
    // during the attach GOP leaves playback a whole GOP behind for good, which
    // is what a reload used to feel like. Small excess is absorbed with a slight
    // speed-up (there is no audio, so it is imperceptible); a large one is a jump.
    function govern() {
      if (!self.playing || gen !== self.gen) return;
      const b = bufferedRange();
      if (!b) return;
      const ahead = b.end - v.currentTime;
      if (ahead > LIVE_HARD_AHEAD) {
        setRate(1);
        seekToEdge();
      } else if (ahead > LIVE_SOFT_AHEAD) {
        setRate(LIVE_MAX_RATE);
      } else {
        setRate(1);
      }
    }

    // The server marks the exact end of the pre-attach GOP, which beats guessing
    // from buffer growth: growth can stall mid-GOP and settle too early.
    function goLive() {
      if (liveStarted || gen !== self.gen) return;
      liveStarted = true;
      releaseTimers();
      seekToEdge();
      startPlayback();
      governorTimer = setInterval(govern, 500);
    }

    function maybeGoLive() {
      if (liveStarted || gen !== self.gen) return;
      if (sb.updating || queue.length) return;
      goLive();
    }

    // Fallback when the edge marker never arrives (lost stream, older server):
    // start on a timer and let the governor close the remaining gap.
    function armAttachWait() {
      if (attachTimer || liveStarted) return;
      attachTimer = setTimeout(function () {
        attachTimer = 0;
        maybeGoLive();
        armAttachWait();
      }, LIVE_ATTACH_WAIT);
    }

    sb.addEventListener("updateend", function () {
      prune();
      flush();
      if (liveStarted) govern();
      else if (backlogComplete && !queue.length && !sb.updating) maybeGoLive();
    });

    if (first && first.length) {
      queue.push(first);
      if (!isInitSeg(first)) {
        sawMedia = true;
        armAttachWait();
      }
      flush();
    }

    while (true) {
      if (gen !== self.gen) { releaseTimers(); return null; }
      const frag = await readFrag(stream);
      if (!frag) { releaseTimers(); return null; }
      if (isLiveEdge(frag)) {
        backlogComplete = true;
        maybeGoLive();
        continue;
      }
      if (isInitSeg(frag) && sawMedia) { releaseTimers(); return frag; }
      queue.push(frag);
      if (!isInitSeg(frag)) {
        sawMedia = true;
        armAttachWait();
      }
      flush();
    }
  }

  global.LivePlayer = LivePlayer;
})(typeof window !== "undefined" ? window : this);
