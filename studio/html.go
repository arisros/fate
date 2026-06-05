package studio

import (
	"fmt"
	"io"
	"strings"
)

// renderShell formats one of the HTML shells and rewrites the "__ASSETVER__"
// token to the current content hash, so asset URLs (e.g. /assets/app.js?v=…)
// bust any CDN/browser cache on every redeploy that changes an asset.
func renderShell(w io.Writer, format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	s = strings.ReplaceAll(s, "__ASSETVER__", assetVersion)
	_, _ = io.WriteString(w, s)
}

// HTML shells. Styling lives in the embedded /assets/app.css; the simulator
// client logic in /assets/app.js; the graph layout engine is the vendored
// /assets/elk.bundled.js. These templates only inject content + JS globals.

// pageShell wraps the index and static /m views. Two %s verbs: title, body.
// Body content is expected to be wrapped by the caller in <div class="index-wrap">.
const pageShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="/assets/app.css?v=__ASSETVER__">
<script>(function(){var t=localStorage.getItem('fate-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');document.documentElement.setAttribute('data-theme',t);})();</script>
</head>
<body>
<div class="index-wrap">
%s
<hr>
<footer><p><small>fate studio</small></p></footer>
</div>
</body>
</html>
`

// welcomeShell is the landing page. Verbs in order: page title, machine-cards
// HTML. The hero copy is statechart-generic so embedders can reuse it; the
// links credit the fate project that powers the studio.
const welcomeShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%[1]s</title>
<link rel="stylesheet" href="/assets/app.css?v=__ASSETVER__">
<script>(function(){var t=localStorage.getItem('fate-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');document.documentElement.setAttribute('data-theme',t);})();</script>
</head>
<body>
<div class="welcome">
  <nav class="topbar">
    <span class="brand">%[1]s</span>
    <span class="spacer"></span>
    <a href="https://github.com/arisros/fate">GitHub</a>
    <a href="https://pkg.go.dev/github.com/arisros/fate">pkg.go.dev</a>
  </nav>

  <header class="hero">
    <p class="eyebrow">Statechart studio</p>
    <h1>Inspect and <span class="chip">simulate</span> your statecharts.</h1>
    <p>A live, browser-based studio for <code>fate</code> machines — explore the chart,
    drive events, fire delayed transitions and invocations, and watch the active
    configuration update in real time over Server-Sent Events.</p>
    <div class="cta-row">
      <a class="btn-primary" href="#machines">Browse machines</a>
      <a class="btn-ghost" href="https://github.com/arisros/fate">View on GitHub</a>
    </div>
    <div class="install"><span class="pmt">$</span> go get github.com/arisros/fate</div>
  </header>

  <h2 class="section-head" id="machines">Demo machines</h2>
  <p class="section-sub">Open any machine to view its diagram, or simulate it live — send events, fire timers, resolve invocations.</p>
  <div class="machines-grid">%[2]s</div>

  <footer>
    <span>fate · a statechart engine for Go</span>
    <a href="https://github.com/arisros/fate">GitHub</a>
    <a href="https://pkg.go.dev/github.com/arisros/fate">Docs</a>
    <a href="https://github.com/arisros/fate/blob/main/LICENSE">MIT</a>
  </footer>
</div>
</body>
</html>
`

// simShell is the full simulator page. Verbs in order:
//
//	1 page title (machine name)
//	2 machine name (header)
//	3 machine name (static-view link)
//	4 JS globals block: window.FATE_MACHINE + window.FATE_EVENTS
const simShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="/assets/app.css?v=__ASSETVER__">
<script>(function(){var t=localStorage.getItem('fate-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');document.documentElement.setAttribute('data-theme',t);})();</script>
</head>
<body class="sim">

<header class="bar">
  <span class="title">fate studio</span>
  <span class="machine">%s</span>
  <span id="status-badge" class="badge">connecting</span>
  <span class="spacer"></span>
  <a class="btn" href="/">index</a>
  <a class="btn" href="/m/%s">static</a>
  <button id="theme-toggle" class="icon-btn" title="toggle theme">◑</button>
  <button id="undo-btn" class="icon-btn" title="step back">↶ undo</button>
  <button id="reset-btn" class="icon-btn" title="reset">⟲ reset</button>
  <button id="import-btn" class="icon-btn" title="import snapshot">⬆ import</button>
  <button id="export-btn" class="icon-btn" title="export snapshot">⬇ export</button>
</header>

<main class="layout">
  <section id="canvas" class="canvas">
    <div id="canvas-inner" class="canvas-inner"></div>
    <button id="fit-btn" class="fit-btn" title="fit to view">⤢ fit</button>
    <span class="hint">scroll = zoom · drag bg = pan · drag header = move node</span>
  </section>
  <aside class="inspector">
    <div>
      <h2>Active state</h2>
      <div class="panel"><code id="state-path">…</code></div>
    </div>
    <div>
      <h2>Events</h2>
      <div id="event-btns"></div>
    </div>
    <div id="effects-panel" hidden>
      <h2>Pending effects</h2>
      <div id="effects"></div>
    </div>
    <div>
      <h2>Context</h2>
      <div class="filter-row"><input id="ctx-filter" placeholder="🔍 filter context…"></div>
      <pre id="context" class="panel">{}</pre>
    </div>
    <div>
      <h2>Timeline</h2>
      <ol id="timeline" class="panel"></ol>
    </div>
  </aside>
</main>

<div id="toasts"></div>

<script>%s</script>
<script src="/assets/elk.bundled.js?v=__ASSETVER__"></script>
<script src="/assets/app.js?v=__ASSETVER__"></script>
</body>
</html>
`
