<template>
  <svg
    class="fate-chart"
    viewBox="0 0 420 300"
    role="img"
    aria-label="An order statechart: pending, paid, shipped, and a cancelled branch"
  >
    <g class="ink">
      <rect class="frame" x="10" y="10" width="400" height="280" rx="30" />
      <path d="M57 110 H90" />
      <rect x="90" y="88" width="100" height="44" rx="13" />
      <path d="M305 132 V200" />
      <rect x="250" y="200" width="110" height="44" rx="13" />
      <path d="M250 222 H232" />
      <circle cx="221" cy="222" r="10" />
    </g>
    <g class="faint">
      <path d="M140 132 V200" />
      <rect x="90" y="200" width="100" height="44" rx="13" />
    </g>
    <path class="taken" d="M190 110 H250" />
    <rect class="lit" x="250" y="88" width="110" height="44" rx="13" />
    <circle class="dot" cx="50" cy="110" r="7.5" />
    <circle class="dot" cx="221" cy="222" r="4.2" />
    <g class="labels">
      <text class="dim region" x="34" y="42">order</text>
      <text x="140" y="115">pending</text>
      <text class="on-lit" x="305" y="115">paid</text>
      <text x="305" y="227">shipped</text>
      <text class="dim" x="140" y="227">cancelled</text>
      <g class="events">
        <text x="220" y="101">PAY</text>
        <text x="330" y="171">SHIP</text>
        <text x="113" y="171">CANCEL</text>
      </g>
    </g>
  </svg>
</template>

<style scoped>
.fate-chart {
  display: block;
  width: 100%;
  height: auto;
}

.ink,
.faint,
.taken {
  fill: none;
  stroke-width: 2.6;
  stroke-linecap: round;
}

.ink {
  stroke: var(--vp-c-text-1);
}

.frame {
  stroke-width: 3.4;
}

.faint {
  stroke: var(--fate-faint);
}

.taken {
  stroke: var(--vp-c-brand-1);
  stroke-width: 3.2;
}

.lit {
  fill: var(--fate-lime);
}

.dot {
  fill: var(--vp-c-text-1);
}

.labels {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  text-anchor: middle;
  fill: var(--vp-c-text-1);
}

.dim {
  fill: var(--vp-c-text-2);
}

.region {
  font-size: 12px;
  text-anchor: start;
}

.on-lit {
  fill: var(--fate-on-lime);
  font-weight: 600;
}

.events {
  font-size: 10.5px;
  letter-spacing: 0.06em;
  fill: var(--vp-c-text-2);
}
</style>
