<template>
  <svg
    class="fate-mark"
    viewBox="0 0 92 64"
    role="img"
    aria-label="The fate mark read as a statechart: an initial state, an active state and a final state inside a compound state"
  >
    <rect class="ink" x="5" y="5" width="54" height="54" rx="13" stroke-width="4.5" />
    <path class="ink" d="M19 46 V26 Q19 19 26 19 H32 M19 40 Q19 35 25 35 H30" stroke-width="4" />
    <circle class="dot" cx="19" cy="47" r="4.3" />
    <rect class="tile" x="31" y="11.5" width="15" height="15" rx="5" />
    <circle class="ink" cx="36" cy="35" r="5" stroke-width="2.6" />
    <circle class="dot" cx="36" cy="35" r="2.1" />
    <path class="lead" d="M47.5 19 H68 M42.5 35 H68 M24.5 47 H68 M55.5 56 H68" />
    <text class="on" x="70" y="19.8">active</text>
    <text x="70" y="35.8">final</text>
    <text x="70" y="47.8">initial</text>
    <text x="70" y="56.8">compound</text>
  </svg>
</template>

<style scoped>
.fate-mark {
  display: block;
  width: 100%;
  height: auto;
}

.ink {
  fill: none;
  stroke: var(--fate-fg);
}

.dot {
  fill: var(--fate-fg);
}

.tile {
  fill: var(--fate-active-line);
}

.lead {
  fill: none;
  stroke: var(--fate-edge);
  stroke-width: 0.4;
  stroke-dasharray: 0.4 1.2;
  stroke-linecap: round;
}

text {
  font-family: var(--vp-font-family-mono);
  font-size: 2.3px;
  fill: var(--fate-muted);
}

.on {
  fill: var(--fate-active-line);
}
</style>
