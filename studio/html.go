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
<script>(function(){var t=localStorage.getItem('scs-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');document.documentElement.setAttribute('data-theme',t);})();</script>
</head>
<body>
<div class="index-wrap">
%s
<hr>
<footer><p><small>scs studio · go-statechart</small></p></footer>
</div>
</body>
</html>
`

// simShell is the full simulator page. Verbs in order:
//
//	1 page title (machine name)
//	2 machine name (header)
//	3 machine name (static-view link)
//	4 JS globals block: window.SCS_MACHINE + window.SCS_EVENTS
const simShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="/assets/app.css?v=__ASSETVER__">
<script>(function(){var t=localStorage.getItem('scs-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');document.documentElement.setAttribute('data-theme',t);})();</script>
</head>
<body class="sim">

<header class="bar">
  <span class="title">scs studio</span>
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
