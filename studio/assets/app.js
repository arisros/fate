/* fate studio — self-hosted Stately-style canvas. Vanilla JS + elkjs layout.
   The graph STRUCTURE is laid out once; SSE only re-highlights. The Go engine
   is authoritative (events POST to /sim/{m}/send). Globals: FATE_MACHINE, FATE_EVENTS. */
(function () {
  "use strict";
  var machine = window.FATE_MACHINE;
  var base = "/sim/" + encodeURIComponent(machine);
  // Node geometry. These MUST match the fixed heights in app.css (.nhead and
  // .erow) so the JS-computed box never clips a row and edge anchors line up.
  var NODE_W = 200, HEAD_H = 28, ROW_H = 22, PAD_TOP = 32;

  var state = {
    nodes: {},        // id -> {node, abs:{x,y,w,h}, el, events:[edge...], el}
    edges: [],        // raw edges
    activePaths: [],  // active leaf dot-paths
    available: window.FATE_EVENTS || [],
    sent: [],
    view: { x: 30, y: 30, scale: 1 },
    pos: {},          // id -> {x,y} manual overrides (localStorage)
    lastCtx: "{}"
  };

  // ---------- theme + toasts ----------
  function curTheme() { return localStorage.getItem("fate-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"); }
  function applyTheme(t) { document.documentElement.setAttribute("data-theme", t); localStorage.setItem("fate-theme", t); }
  function toast(msg, kind) {
    var box = document.getElementById("toasts"), el = document.createElement("div");
    el.className = "toast " + (kind || ""); el.textContent = msg; box.appendChild(el);
    setTimeout(function () { el.style.opacity = "0"; setTimeout(function () { el.remove(); }, 200); }, 3500);
  }

  // ---------- graph load + layout ----------
  function loadPos() { try { return JSON.parse(localStorage.getItem("fate-pos-" + machine) || "{}"); } catch (_) { return {}; } }
  function savePos() { localStorage.setItem("fate-pos-" + machine, JSON.stringify(state.pos)); }

  // Persistent status line inside the canvas (NOT a fading toast) so layout
  // failures are always visible instead of leaving a blank screen.
  function status(msg, kind) {
    var el = document.getElementById("canvas-status");
    if (!el) {
      el = document.createElement("div"); el.id = "canvas-status";
      var c = document.getElementById("canvas"); if (c) c.appendChild(el);
    }
    if (!msg) { el.style.display = "none"; return; }
    el.style.display = "block"; el.className = kind || ""; el.textContent = msg;
  }

  function nodeBox(n) { return { w: NODE_W, h: HEAD_H + n.events.length * ROW_H + 8 }; }
  function headerH(n) { return PAD_TOP + n.events.length * ROW_H; }

  function buildAndLayout() {
    status("loading graph…");
    return fetch("/m/" + encodeURIComponent(machine) + "/graph")
      .then(function (r) { if (!r.ok) throw new Error("graph HTTP " + r.status); return r.json(); })
      .then(function (graph) {
        prepareGraph(graph);
        status("computing layout…");
        return layoutGraph(graph).then(function () { status(""); render(); });
      });
  }

  // prepareGraph indexes nodes, links children, and attaches the distinct
  // outgoing events to each node (in first-seen order).
  function prepareGraph(graph) {
    var byId = {};
    graph.nodes.forEach(function (n) { byId[n.id] = Object.assign({}, n, { children: [], events: [] }); });
    graph.nodes.forEach(function (n) { if (n.parent && byId[n.parent]) byId[n.parent].children.push(byId[n.id]); });
    var seen = {};
    graph.edges.forEach(function (e) {
      var k = e.source + "|" + e.event;
      if (!seen[k]) { seen[k] = true; if (byId[e.source]) byId[e.source].events.push(e); }
    });
    state.edges = graph.edges;
    state.byId = byId;
    state.roots = graph.nodes.filter(function (n) { return !n.parent; }).map(function (n) { return byId[n.id]; });
    state.pos = loadPos();
  }

  // layoutGraph tries elkjs (nicest hierarchical layout) but NEVER lets a
  // failure blank the canvas: a 5s watchdog + try/catch + the elk error path
  // all fall back to the deterministic built-in layout. Resolves once
  // state.nodes is populated by whichever engine won.
  function layoutGraph(graph) {
    return new Promise(function (resolve) {
      var done = false;
      function finish() { if (done) return; done = true; resolve(); }
      var wd = setTimeout(function () {
        if (done) return;
        status("layout engine slow — using built-in layout", "warn");
        fallbackLayout(); finish();
      }, 5000);
      function useFallback(why) {
        if (done) return; clearTimeout(wd);
        status("built-in layout (" + why + ")", "warn");
        fallbackLayout(); finish();
      }
      try {
        if (typeof ELK === "undefined") { useFallback("elk not loaded"); return; }
        var elk = new ELK();
        elk.layout(toElkGraph(graph)).then(function (laid) {
          if (done) return; clearTimeout(wd);
          state.nodes = {};
          computeAbs(laid, 0, 0);
          if (!Object.keys(state.nodes).length) { useFallback("elk produced no nodes"); return; }
          finish();
        }).catch(function (e) { useFallback("elk: " + (e && e.message || e)); });
      } catch (e) { useFallback("elk: " + (e && e.message || e)); }
    });
  }

  function toElkGraph(graph) {
    function toElk(n) {
      var en = { id: n.id, labels: [{ text: n.label }] };
      if (n.children.length) {
        en.children = n.children.map(toElk);
        en.layoutOptions = { "elk.padding": "[top=" + headerH(n) + ",left=14,bottom=14,right=14]" };
      } else {
        var b = nodeBox(n); en.width = b.w; en.height = b.h;
      }
      return en;
    }
    return {
      id: "root",
      layoutOptions: {
        "elk.algorithm": "layered", "elk.direction": "DOWN",
        "elk.hierarchyHandling": "INCLUDE_CHILDREN",
        "elk.layered.spacing.nodeNodeBetweenLayers": "46",
        "elk.spacing.nodeNode": "28", "elk.spacing.edgeNode": "18",
        "elk.layered.spacing.edgeNodeBetweenLayers": "18"
      },
      children: state.roots.map(toElk),
      edges: graph.edges.map(function (e) { return { id: e.id, sources: [e.source], targets: [e.target] }; })
    };
  }

  function computeAbs(elkNode, ox, oy) {
    if (elkNode.id !== "root") {
      var ax = ox + (elkNode.x || 0), ay = oy + (elkNode.y || 0);
      var n = state.byId[elkNode.id];
      if (n) {
        // honor manual override
        if (state.pos[elkNode.id]) { ax = state.pos[elkNode.id].x; ay = state.pos[elkNode.id].y; }
        var w = elkNode.width, h = elkNode.height;
        if (!(w > 0)) w = NODE_W;                                   // guard: never NaN/0 → fit() stays finite
        if (!(h > 0)) h = HEAD_H + (n.events.length * ROW_H) + 6;
        state.nodes[elkNode.id] = { meta: n, x: ax, y: ay, w: w, h: h };
      }
      ox = ox + (elkNode.x || 0); oy = oy + (elkNode.y || 0);
      // if overridden, children follow the override delta
      if (n && state.pos[elkNode.id]) {
        ox = state.pos[elkNode.id].x; oy = state.pos[elkNode.id].y;
      }
    }
    (elkNode.children || []).forEach(function (c) { computeAbs(c, ox, oy); });
  }

  // fallbackLayout: deterministic, dependency-free hierarchical layout. Each
  // container wraps its children in a left-to-right flow (max 3 columns) below
  // a header band; leaves are fixed-size cards. Guarantees a non-empty canvas
  // and finite coordinates even when elk is unavailable. Fully node-testable.
  function fallbackLayout() {
    var GAP = 26, MAXCOL = 3;
    state.nodes = {};
    // measure: returns {w,h} and records each child's relative offset on the node
    function measure(n) {
      if (!n.children.length) { var b = nodeBox(n); n._rel = []; return { w: b.w, h: b.h }; }
      var sized = n.children.map(function (c) { return { c: c, box: measure(c) }; });
      var top = headerH(n), x = 14, y = top, rowH = 0, maxW = 0, col = 0;
      var rels = [];
      sized.forEach(function (s) {
        if (col >= MAXCOL) { x = 14; y += rowH + GAP; rowH = 0; col = 0; }
        rels.push({ id: s.c.id, dx: x, dy: y });
        x += s.box.w + GAP; rowH = Math.max(rowH, s.box.h); maxW = Math.max(maxW, x - GAP); col++;
      });
      n._rel = rels;
      return { w: Math.max(NODE_W, maxW + 14), h: y + rowH + 14 };
    }
    // place: assign absolute positions walking the offsets
    function place(n, x, y, box) {
      if (state.pos[n.id]) { x = state.pos[n.id].x; y = state.pos[n.id].y; }
      state.nodes[n.id] = { meta: n, x: x, y: y, w: box.w, h: box.h };
      (n._rel || []).forEach(function (r) {
        var child = state.byId[r.id];
        place(child, x + r.dx, y + r.dy, measure(child));
      });
    }
    // lay roots out in a top-level flow
    var rx = 0, ry = 0, rowH = 0, col = 0;
    state.roots.forEach(function (n) {
      var box = measure(n);
      if (col >= MAXCOL) { rx = 0; ry += rowH + GAP * 2; rowH = 0; col = 0; }
      place(n, rx, ry, box);
      rx += box.w + GAP * 2; rowH = Math.max(rowH, box.h); col++;
    });
  }

  // ---------- render ----------
  function render() {
    var inner = document.getElementById("canvas-inner");
    inner.innerHTML = "";
    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.id = "edges"; svg.setAttribute("width", "10000"); svg.setAttribute("height", "10000");
    svg.innerHTML = '<defs><marker id="arr" markerWidth="9" markerHeight="9" refX="7" refY="3" orient="auto">' +
      '<path d="M0,0 L7,3 L0,6 Z" fill="var(--edge)"/></marker>' +
      '<marker id="arrA" markerWidth="9" markerHeight="9" refX="7" refY="3" orient="auto">' +
      '<path d="M0,0 L7,3 L0,6 Z" fill="var(--edge-active)"/></marker></defs>';
    inner.appendChild(svg);

    // containers first (so leaves stack above), then leaves
    var order = Object.keys(state.nodes).sort(function (a, b) {
      var ca = state.nodes[a].meta.children ? state.nodes[a].meta.children.length : 0;
      var cb = state.nodes[b].meta.children ? state.nodes[b].meta.children.length : 0;
      return (cb > 0) - (ca > 0);
    });
    order.forEach(function (id) { inner.appendChild(nodeEl(state.nodes[id])); });

    drawEdges(svg);
    applyView();
    updateActive();
  }

  function nodeEl(N) {
    var m = N.meta, isC = m.children && m.children.length;
    var el = document.createElement("div");
    el.className = "node" + (isC ? " container" : "") + (m.type === "parallel" ? " parallel" : "") + (m.type === "final" ? " final" : "");
    el.dataset.path = m.path; el.dataset.id = m.id;
    el.style.left = N.x + "px"; el.style.top = N.y + "px";
    el.style.width = N.w + "px"; el.style.height = N.h + "px";

    var head = document.createElement("div");
    head.className = "nhead";
    var dot = "";
    if (m.initial) dot = '<span class="dot" title="initial"></span>';
    var ic = "";
    if (m.type === "parallel") ic = '<span class="ic">▥ parallel</span>';
    else if (m.type === "final") ic = '<span class="ic">◉ final</span>';
    else if (m.type === "history") ic = '<span class="ic">H' + (m.history === "deep" ? "*" : "") + '</span>';
    head.innerHTML = dot + '<span class="nm"></span>' + ic;
    head.querySelector(".nm").textContent = m.label;
    el.appendChild(head);

    // event rows
    (m.events || []).forEach(function (e) {
      var row = document.createElement("div");
      row.className = "erow"; row.dataset.event = e.event;
      var tgt = state.byId[e.target] ? state.byId[e.target].label : "?";
      var gd = e.guard ? '<span class="gd">[' + esc(e.guard) + ']</span>' : "";
      row.innerHTML = '<span class="ev"></span>' + gd + '<span class="tg">→ ' + esc(tgt) + (e.internal ? " ⟳" : "") + '</span>';
      row.querySelector(".ev").textContent = e.event;
      row.onclick = function (ev) {
        ev.stopPropagation();
        if (row.classList.contains("sendable")) sendEvent(e.event);
      };
      el.appendChild(row);
    });

    makeDraggable(el, N);
    return el;
  }

  function rowAnchorY(N, event) {
    // y offset of the event row within the node (header + index*ROW_H + half)
    var idx = (N.meta.events || []).findIndex(function (e) { return e.event === event; });
    if (idx < 0) return N.y + HEAD_H / 2;
    var top = N.meta.children && N.meta.children.length ? PAD_TOP : HEAD_H;
    return N.y + top + idx * ROW_H + ROW_H / 2;
  }

  function drawEdges(svg) {
    var seen = {};
    state.edges.forEach(function (e) {
      var k = e.source + "|" + e.event + "|" + e.target;
      if (seen[k]) return; seen[k] = true;
      var S = state.nodes[e.source], T = state.nodes[e.target];
      if (!S || !T) return;
      var p = document.createElementNS("http://www.w3.org/2000/svg", "path");
      p.dataset.source = e.source; p.dataset.target = e.target;
      var d, x1, y1, x2, y2;
      if (e.source === e.target) { // self loop
        x1 = S.x + S.w; y1 = rowAnchorY(S, e.event);
        d = "M" + x1 + "," + y1 + " C" + (x1 + 34) + "," + (y1 - 16) + " " + (x1 + 34) + "," + (y1 + 16) + " " + x1 + "," + (y1 + 2);
      } else {
        x1 = S.x + S.w; y1 = rowAnchorY(S, e.event);
        x2 = T.x; y2 = T.y + T.h / 2;
        if (x2 < x1) { x2 = T.x + T.w; } // target to the left → exit its right side visually
        var dx = Math.max(30, Math.abs(x2 - x1) / 2);
        d = "M" + x1 + "," + y1 + " C" + (x1 + dx) + "," + y1 + " " + (x2 - dx) + "," + y2 + " " + x2 + "," + y2;
      }
      p.setAttribute("d", d); p.setAttribute("marker-end", "url(#arr)");
      svg.appendChild(p);
    });
  }

  // ---------- drag / pan / zoom ----------
  function makeDraggable(el, N) {
    var head = el.querySelector(".nhead");
    head.onmousedown = function (e) {
      e.preventDefault(); e.stopPropagation();
      var sx = e.clientX, sy = e.clientY, ox = N.x, oy = N.y;
      function mv(ev) {
        var dx = (ev.clientX - sx) / state.view.scale, dy = (ev.clientY - sy) / state.view.scale;
        N.x = ox + dx; N.y = oy + dy;
        el.style.left = N.x + "px"; el.style.top = N.y + "px";
        state.pos[N.meta.id] = { x: N.x, y: N.y };
        redrawEdges();
      }
      function up() { window.removeEventListener("mousemove", mv); window.removeEventListener("mouseup", up); savePos(); }
      window.addEventListener("mousemove", mv); window.addEventListener("mouseup", up);
    };
  }
  function redrawEdges() {
    var svg = document.getElementById("edges");
    if (!svg) return;
    // keep defs, drop paths
    Array.prototype.slice.call(svg.querySelectorAll("path[data-source]")).forEach(function (p) { p.remove(); });
    drawEdges(svg);
    updateActive();
  }
  function applyView() {
    var inner = document.getElementById("canvas-inner");
    inner.style.transform = "translate(" + state.view.x + "px," + state.view.y + "px) scale(" + state.view.scale + ")";
  }
  function wireCanvas() {
    var canvas = document.getElementById("canvas");
    canvas.onwheel = function (e) {
      e.preventDefault();
      var f = e.deltaY < 0 ? 1.1 : 0.9, ns = Math.min(2.5, Math.max(0.25, state.view.scale * f));
      var rect = canvas.getBoundingClientRect(), mx = e.clientX - rect.left, my = e.clientY - rect.top;
      state.view.x = mx - (mx - state.view.x) * (ns / state.view.scale);
      state.view.y = my - (my - state.view.y) * (ns / state.view.scale);
      state.view.scale = ns; applyView();
    };
    canvas.onmousedown = function (e) {
      if (e.target.closest(".node")) return; // node drag handled separately
      var sx = e.clientX, sy = e.clientY, ox = state.view.x, oy = state.view.y;
      function mv(ev) { state.view.x = ox + (ev.clientX - sx); state.view.y = oy + (ev.clientY - sy); applyView(); }
      function up() { window.removeEventListener("mousemove", mv); window.removeEventListener("mouseup", up); }
      window.addEventListener("mousemove", mv); window.addEventListener("mouseup", up);
    };
    document.getElementById("fit-btn").onclick = fit;
  }
  function fit() {
    var xs = [], ys = [], xe = [], ye = [];
    Object.keys(state.nodes).forEach(function (id) {
      var n = state.nodes[id];
      if (isFinite(n.x) && isFinite(n.y) && isFinite(n.w) && isFinite(n.h)) {
        xs.push(n.x); ys.push(n.y); xe.push(n.x + n.w); ye.push(n.y + n.h);
      }
    });
    if (!xs.length) { state.view = { x: 30, y: 30, scale: 1 }; applyView(); return; }
    var minX = Math.min.apply(0, xs), minY = Math.min.apply(0, ys), maxX = Math.max.apply(0, xe), maxY = Math.max.apply(0, ye);
    var canvas = document.getElementById("canvas"), cw = canvas.clientWidth || 800, ch = canvas.clientHeight || 600;
    var s = Math.min(cw / (maxX - minX + 80), ch / (maxY - minY + 80), 1.4);
    if (!(s > 0) || !isFinite(s)) s = 1;                            // never NaN → never blank
    state.view.scale = s; state.view.x = (cw - (maxX - minX) * s) / 2 - minX * s; state.view.y = 24 - minY * s;
    if (!isFinite(state.view.x)) state.view.x = 30;
    if (!isFinite(state.view.y)) state.view.y = 30;
    applyView();
  }

  // ---------- active highlight (per SSE snapshot — no relayout) ----------
  function updateActive() {
    var leaves = state.activePaths;
    var activeSet = {};
    leaves.forEach(function (p) {
      var parts = p.split(".");
      for (var i = 1; i <= parts.length; i++) activeSet[parts.slice(0, i).join(".")] = true;
    });
    document.querySelectorAll(".node").forEach(function (el) {
      var on = !!activeSet[el.dataset.path];
      el.classList.toggle("active", on);
      // sendable rows: on active leaf nodes whose event is currently available
      el.querySelectorAll(".erow").forEach(function (row) {
        var leaf = leaves.indexOf(el.dataset.path) >= 0;
        var avail = state.available.indexOf(row.dataset.event) >= 0;
        row.classList.toggle("sendable", leaf && avail);
      });
    });
    // active edges: source path active and target path active-or-reachable
    var svg = document.getElementById("edges");
    if (svg) svg.querySelectorAll("path[data-source]").forEach(function (p) {
      var srcN = state.nodes[p.dataset.source];
      var act = srcN && activeSet[srcN.meta.path];
      p.classList.toggle("active", !!act);
      p.setAttribute("marker-end", act ? "url(#arrA)" : "url(#arr)");
    });
  }

  // ---------- inspector ----------
  function applySnapshot(snap) {
    document.getElementById("state-path").textContent = snap.path;
    setBadge(snap.status);
    state.activePaths = (snap.path || "").split(" | ").map(function (s) { return s.trim(); }).filter(Boolean);
    // snap.context is the engine's raw JSON context: because the whole SSE
    // payload is already JSON.parse'd, it arrives as a parsed value (object /
    // array / scalar), NOT a string. Keep it as-is; renderContext pretty-prints
    // it. (Re-parsing a parsed object would coerce it to "[object Object]".)
    state.lastCtx = (snap.context === undefined || snap.context === null) ? {} : snap.context;
    renderContext(); renderEffects(snap.timers || [], snap.invocations || []); updateActive();
  }

  // renderEffects shows pending delayed transitions (with a Fire button) and
  // pending invocations (with Resolve / Reject controls) — the timer/invoke
  // visualization. Hidden entirely when the machine has no pending effects.
  function renderEffects(timers, invokes) {
    var panel = document.getElementById("effects-panel");
    var box = document.getElementById("effects");
    box.innerHTML = "";
    if (!timers.length && !invokes.length) { panel.hidden = true; return; }
    panel.hidden = false;
    timers.forEach(function (t) {
      var row = document.createElement("div"); row.className = "effect timer";
      row.innerHTML = '<span class="ekind">⏲ after</span><span class="edelay"></span>';
      row.querySelector(".edelay").textContent = t.delay;
      var fire = document.createElement("button"); fire.className = "evt-btn"; fire.textContent = "fire ▶";
      fire.onclick = function () { fireTimer(t.id); };
      row.appendChild(fire); box.appendChild(row);
    });
    invokes.forEach(function (iv) {
      var row = document.createElement("div"); row.className = "effect invoke";
      row.innerHTML = '<span class="ekind">⮞ invoke</span><span class="esrc"></span>';
      row.querySelector(".esrc").textContent = iv.src;
      var out = document.createElement("input"); out.className = "einput"; out.placeholder = 'output JSON (e.g. true, 42, {"k":1})';
      var ok = document.createElement("button"); ok.className = "evt-btn ok"; ok.textContent = "resolve ✓";
      ok.onclick = function () { resolveInvoke(iv.id, out.value); };
      var no = document.createElement("button"); no.className = "evt-btn err"; no.textContent = "reject ✗";
      no.onclick = function () { rejectInvoke(iv.id); };
      var ctrls = document.createElement("div"); ctrls.className = "ectrls";
      ctrls.appendChild(out); ctrls.appendChild(ok); ctrls.appendChild(no);
      row.appendChild(ctrls); box.appendChild(row);
    });
  }
  function setBadge(s) { var b = document.getElementById("status-badge"); b.textContent = s; b.className = "badge " + s; }
  function renderContext() {
    var pre = document.getElementById("context"), q = document.getElementById("ctx-filter").value.trim(), pretty;
    var val = state.lastCtx;
    if (typeof val === "string") {
      try { pretty = JSON.stringify(JSON.parse(val), null, 2); } catch (_) { pretty = val; }
    } else {
      try { pretty = JSON.stringify(val, null, 2); } catch (_) { pretty = String(val); }
    }
    if (!q) { pre.textContent = pretty; return; }
    var e = pretty.replace(/[&<>]/g, function (c) { return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]; });
    var re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi");
    pre.innerHTML = e.replace(re, function (m) { return "<span class='hit'>" + m + "</span>"; });
  }
  function renderEvents(events) {
    state.available = events || [];
    var div = document.getElementById("event-btns"); div.innerHTML = "";
    if (!state.available.length) { div.textContent = "(no events — machine at rest)"; updateActive(); return; }
    state.available.forEach(function (ev) {
      var b = document.createElement("button"); b.className = "evt-btn"; b.textContent = ev;
      b.onclick = function () { sendEvent(ev); }; div.appendChild(b);
    });
    updateActive();
  }

  // ---------- actions ----------
  function sendEvent(ev) {
    fetch(base + "/send", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: "event=" + encodeURIComponent(ev) })
      .then(function (r) { if (!r.ok) return r.text().then(function (t) { throw new Error(t || ("HTTP " + r.status)); }); return r.json(); })
      .then(function (snap) {
        state.sent.push(ev); updateHash(); addTimeline(ev, snap.path); renderEvents(snap.events);
      }).catch(function (err) { toast("send " + ev + ": " + err.message, "error"); });
  }
  function fireTimer(id) {
    post(base + "/timer", "id=" + encodeURIComponent(id), "timer")
      .then(function (snap) { addTimeline("⏲ after", snap.path); renderEvents(snap.events); })
      .catch(function (err) { toast("fire timer: " + err.message, "error"); });
  }
  function resolveInvoke(id, output) {
    post(base + "/invoke", "id=" + encodeURIComponent(id) + "&action=resolve&output=" + encodeURIComponent(output || ""), "resolve")
      .then(function (snap) { addTimeline("✓ resolve", snap.path); renderEvents(snap.events); })
      .catch(function (err) { toast("resolve: " + err.message, "error"); });
  }
  function rejectInvoke(id) {
    post(base + "/invoke", "id=" + encodeURIComponent(id) + "&action=reject", "reject")
      .then(function (snap) { addTimeline("✗ reject", snap.path); renderEvents(snap.events); })
      .catch(function (err) { toast("reject: " + err.message, "error"); });
  }
  // post is a small helper: POST a urlencoded body, parse JSON, throw on !ok.
  function post(url, body, label) {
    return fetch(url, { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: body })
      .then(function (r) { if (!r.ok) return r.text().then(function (t) { throw new Error(t || (label + " HTTP " + r.status)); }); return r.json(); });
  }
  function undo() {
    fetch(base + "/undo", { method: "POST" }).then(function (r) { if (!r.ok) return r.text().then(function (t) { throw new Error(t); }); return r.json(); })
      .then(function (snap) { state.sent.pop(); updateHash(); var ol = document.getElementById("timeline"); if (ol.firstChild) ol.removeChild(ol.firstChild); renderEvents(snap.events); })
      .catch(function (err) { toast("undo: " + err.message, "error"); });
  }
  function resetSim() {
    fetch(base + "/reset", { method: "POST" }).then(function (r) { return r.json(); })
      .then(function (snap) { state.sent = []; updateHash(); document.getElementById("timeline").innerHTML = ""; renderEvents(snap.events); toast("reset", "ok"); })
      .catch(function (err) { toast("reset: " + err.message, "error"); });
  }
  function exportSnap() { window.location = base + "/export"; }
  function importSnap() {
    var inp = document.createElement("input"); inp.type = "file"; inp.accept = "application/json";
    inp.onchange = function () {
      var f = inp.files[0]; if (!f) return;
      f.text().then(function (txt) { return fetch(base + "/import", { method: "POST", headers: { "Content-Type": "application/json" }, body: txt }); })
        .then(function (r) { if (!r.ok) return r.text().then(function (t) { throw new Error(t); }); return r.json(); })
        .then(function (snap) { state.sent = []; updateHash(); document.getElementById("timeline").innerHTML = ""; renderEvents(snap.events); toast("imported", "ok"); })
        .catch(function (err) { toast("import: " + err.message, "error"); });
    };
    inp.click();
  }
  function addTimeline(ev, path) {
    var ol = document.getElementById("timeline"), li = document.createElement("li");
    li.innerHTML = "<span class='ev'></span><span class='arrow'>→ " + esc((path || "").split(" | ")[0]) + "</span>";
    li.querySelector(".ev").textContent = ev; ol.insertBefore(li, ol.firstChild);
  }
  function updateHash() { history.replaceState(null, "", state.sent.length ? "#e=" + state.sent.join(",") : "#"); }
  function replayHash() {
    var m = (location.hash || "").match(/#e=([^&]+)/); if (!m) return;
    var seq = decodeURIComponent(m[1]).split(",").filter(Boolean);
    fetch(base + "/reset", { method: "POST" }).then(function () {
      (function step(i) {
        if (i >= seq.length) { toast("replayed " + seq.length, "ok"); return; }
        fetch(base + "/send", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: "event=" + encodeURIComponent(seq[i]) })
          .then(function (r) { return r.ok ? r.json() : null; }).then(function (snap) { if (snap) { state.sent.push(seq[i]); addTimeline(seq[i], snap.path); renderEvents(snap.events); } step(i + 1); });
      })(0);
    });
  }

  // ---------- SSE ----------
  function connectSSE() {
    var es = new EventSource(base + "/stream");
    es.onmessage = function (e) { applySnapshot(JSON.parse(e.data)); };
    es.onerror = function () { setBadge("disconnected"); };
  }

  function esc(s) { return String(s).replace(/[&<>]/g, function (c) { return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]; }); }

  // ---------- bootstrap ----------
  document.addEventListener("DOMContentLoaded", function () {
    applyTheme(curTheme());
    document.getElementById("theme-toggle").onclick = function () { applyTheme(curTheme() === "dark" ? "light" : "dark"); };
    document.getElementById("reset-btn").onclick = resetSim;
    document.getElementById("undo-btn").onclick = undo;
    document.getElementById("import-btn").onclick = importSnap;
    document.getElementById("export-btn").onclick = exportSnap;
    document.getElementById("ctx-filter").oninput = renderContext;
    wireCanvas();
    renderEvents(state.available);
    buildAndLayout().then(function () { fit(); }).catch(function (err) { toast("layout: " + err.message, "error"); });
    connectSSE();
    if (location.hash.indexOf("#e=") === 0) setTimeout(replayHash, 400);
  });
})();
