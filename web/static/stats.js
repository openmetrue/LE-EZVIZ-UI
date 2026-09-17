// Battery history chart for the Device tab. The x-axis is pinned to the
// selected window (day / week / month) so switching range is visible even
// before there is older data. No dependencies.
//
// /api/stats returns Unix SECONDS; the canvas works in milliseconds, so every
// point is scaled once on load to keep the domain and the samples in one unit.
(function (global) {
  "use strict";

  var RANGES = {
    day: 24 * 3600 * 1000,
    week: 7 * 24 * 3600 * 1000,
    month: 30 * 24 * 3600 * 1000,
  };

  function init(cfg) {
    cfg = cfg || {};
    var base = cfg.base || "";
    var loc = cfg.locale || "en";
    var pollMin = cfg.pollMin || 15;
    var gapSec = cfg.gapSec || 2700;
    var t = cfg.strings || {};

    var canvas = document.getElementById("chart");
    var wrap = document.getElementById("chartwrap");
    var tip = document.getElementById("ctip");
    var meta = document.getElementById("meta");
    var current = document.getElementById("current");
    if (!canvas || !wrap) return;

    var range = "day";
    var pts = [];
    var hover = null;
    var domainEnd = Date.now();
    var domainStart = domainEnd - RANGES.day;
    var timer = 0;

    var buttons = document.querySelectorAll("#rangeSeg button");
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].addEventListener("click", onRange);
    }

    function onRange(e) {
      var next = e.currentTarget.getAttribute("data-r");
      if (!RANGES[next] || next === range) return;
      range = next;
      for (var j = 0; j < buttons.length; j++) {
        buttons[j].classList.toggle("on", buttons[j] === e.currentTarget);
      }
      hover = null;
      tip.style.display = "none";
      load();
    }

    function formatAxis(ts) {
      var d = new Date(ts);
      if (range === "day") {
        return d.toLocaleTimeString(loc, { hour: "2-digit", minute: "2-digit" });
      }
      return d.toLocaleDateString(loc, { day: "2-digit", month: "2-digit" });
    }

    function formatHover(ts) {
      return new Date(ts).toLocaleString(loc, {
        day: "2-digit", month: "2-digit",
        hour: "2-digit", minute: "2-digit",
      });
    }

    function geom() {
      var W = canvas.clientWidth || 320;
      var H = canvas.clientHeight || 240;
      var padL = 34, padR = 10, padT = 12, padB = 22;
      var iw = Math.max(1, W - padL - padR);
      var ih = Math.max(1, H - padT - padB);
      var span = Math.max(1, domainEnd - domainStart);
      return {
        W: W, H: H, padL: padL, padR: padR, padT: padT, padB: padB,
        iw: iw, ih: ih,
        px: function (ts) { return padL + iw * (ts - domainStart) / span; },
        py: function (v) { return padT + ih * (1 - Math.max(0, Math.min(100, v)) / 100); },
      };
    }

    function drawGrid(ctx, g) {
      ctx.strokeStyle = "#2a3038";
      ctx.fillStyle = "#9aa4b2";
      ctx.font = "11px sans-serif";
      ctx.lineWidth = 1;
      for (var p = 0; p <= 100; p += 25) {
        var y = g.padT + g.ih * (1 - p / 100);
        ctx.beginPath();
        ctx.moveTo(g.padL, y);
        ctx.lineTo(g.W - g.padR, y);
        ctx.stroke();
        ctx.fillText(p + "%", 2, y + 4);
      }
      var ticks = range === "day" ? 4 : (range === "week" ? 7 : 6);
      for (var i = 0; i <= ticks; i++) {
        var ts = domainStart + (domainEnd - domainStart) * i / ticks;
        var x = g.px(ts);
        ctx.beginPath();
        ctx.moveTo(x, g.padT);
        ctx.lineTo(x, g.padT + g.ih);
        ctx.stroke();
      }
      for (var k = 0; k <= ticks; k += 2) {
        var tsk = domainStart + (domainEnd - domainStart) * k / ticks;
        var label = formatAxis(tsk);
        var lx = g.px(tsk);
        var w = ctx.measureText(label).width;
        if (lx - w / 2 < 2) lx = w / 2 + 2;
        if (lx + w / 2 > g.W - 2) lx = g.W - w / 2 - 2;
        ctx.fillText(label, lx - w / 2, g.H - 6);
      }
    }

    function drawLine(ctx, g) {
      if (!pts.length) return;
      var i, s, p;
      var segs = [];
      var cur = [pts[0]];
      for (i = 1; i < pts.length; i++) {
        if (pts[i].ts - pts[i - 1].ts > gapSec * 1000) {
          segs.push(cur);
          cur = [];
        }
        cur.push(pts[i]);
      }
      segs.push(cur);

      for (s = 0; s < segs.length; s++) {
        var seg = segs[s];
        if (seg.length < 2) continue;
        ctx.beginPath();
        ctx.moveTo(g.px(seg[0].ts), g.py(seg[0].b));
        for (p = 1; p < seg.length; p++) {
          ctx.lineTo(g.px(seg[p].ts), g.py(seg[p].b));
        }
        ctx.strokeStyle = "#34d399";
        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.lineTo(g.px(seg[seg.length - 1].ts), g.padT + g.ih);
        ctx.lineTo(g.px(seg[0].ts), g.padT + g.ih);
        ctx.closePath();
        ctx.fillStyle = "rgba(52,211,153,0.10)";
        ctx.fill();
      }

      for (i = 0; i < pts.length; i++) {
        p = pts[i];
        ctx.beginPath();
        ctx.arc(g.px(p.ts), g.py(p.b), i === hover ? 5 : 3, 0, Math.PI * 2);
        ctx.fillStyle = "#34d399";
        ctx.fill();
        if (i === hover) {
          ctx.lineWidth = 2;
          ctx.strokeStyle = "#fff";
          ctx.stroke();
        }
      }
    }

    function draw() {
      var dpr = global.devicePixelRatio || 1;
      var g = geom();
      canvas.width = g.W * dpr;
      canvas.height = g.H * dpr;
      var ctx = canvas.getContext("2d");
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, g.W, g.H);
      drawGrid(ctx, g);
      if (!pts.length) {
        ctx.fillStyle = "#9aa4b2";
        ctx.font = "13px sans-serif";
        var msg = (t.empty || "No battery data yet — the first sample appears within %s minutes.");
        msg = msg.replace("%s", pollMin);
        var mw = ctx.measureText(msg).width;
        ctx.fillText(msg, Math.max(4, (g.W - mw) / 2), g.padT + g.ih / 2);
        return;
      }
      drawLine(ctx, g);
    }

    function renderHead() {
      var last = pts.length ? pts[pts.length - 1] : null;
      if (current) {
        if (!last) {
          current.textContent = "—";
        } else {
          var txt = (t.battery || "Battery") + ": " + last.b + "% · " +
            (last.on ? (t.online || "online") : (t.offline || "offline"));
          current.textContent = txt;
        }
      }
      if (meta) {
        meta.textContent = pts.length
          ? (t.points || "").replace("%s", String(pts.length)).replace("%s", pollMin)
          : (t.empty || "").replace("%s", pollMin);
      }
    }

    function nearest(mx) {
      if (!pts.length) return null;
      var g = geom();
      if (mx < g.padL - 4 || mx > g.W - g.padR + 4) return null;
      var best = 0, bestD = Infinity;
      for (var i = 0; i < pts.length; i++) {
        var d = Math.abs(g.px(pts[i].ts) - mx);
        if (d < bestD) { bestD = d; best = i; }
      }
      return best;
    }

    function showTip(i, mx, my) {
      var p = pts[i];
      tip.innerHTML = "<b>" + p.b + "%</b> · " + formatHover(p.ts) + " · " +
        (p.on ? (t.online || "online") : (t.offline || "offline"));
      tip.style.display = "block";
      var tw = tip.offsetWidth, th = tip.offsetHeight, W = wrap.clientWidth;
      var left = mx + 14;
      if (left + tw > W - 4) left = mx - tw - 10;
      if (left < 4) left = 4;
      var top = my - th - 10;
      if (top < 4) top = my + 14;
      tip.style.left = left + "px";
      tip.style.top = top + "px";
    }

    canvas.addEventListener("pointermove", function (e) {
      var r = canvas.getBoundingClientRect();
      var mx = e.clientX - r.left, my = e.clientY - r.top;
      var i = nearest(mx);
      if (i == null) {
        if (hover != null) { hover = null; draw(); }
        tip.style.display = "none";
        return;
      }
      var changed = hover !== i;
      hover = i;
      if (changed) draw();
      showTip(i, mx, my);
    });

    canvas.addEventListener("pointerleave", function () {
      hover = null;
      tip.style.display = "none";
      draw();
    });

    var resizeTimer = 0;
    global.addEventListener("resize", function () {
      if (resizeTimer) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(function () { if (pts.length) draw(); }, 120);
    });

    function schedule() {
      if (timer) clearInterval(timer);
      var sec = Math.max(30, pollMin * 60);
      timer = setInterval(function () {
        if (!document.hidden) load();
      }, sec * 1000);
    }

    function load() {
      domainEnd = Date.now();
      domainStart = domainEnd - RANGES[range];
      fetch(base + "/api/stats?range=" + range, { cache: "no-store" })
        .then(function (r) { return r.json(); })
        .then(function (data) {
          var raw = Array.isArray(data) ? data : [];
          pts = raw.map(function (p) {
            return { ts: p.ts * 1000, b: p.b, on: p.on };
          });
          hover = null;
          tip.style.display = "none";
          draw();
          renderHead();
        })
        .catch(function () {
          pts = [];
          draw();
          if (meta) meta.textContent = t.fail || "Could not load battery history.";
        });
    }

    schedule();
    load();
  }

  global.StatsChart = { init: init };
})(typeof window !== "undefined" ? window : this);
