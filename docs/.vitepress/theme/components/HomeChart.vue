<script setup lang="ts">
import { computed, ref } from 'vue'

type StateId = 'pending' | 'paid' | 'shipped' | 'delivered' | 'cancelled'

const transitions: Partial<Record<StateId, Record<string, StateId>>> = {
  pending: { PAY: 'paid', CANCEL: 'cancelled' },
  paid: { SHIP: 'shipped' },
  shipped: { DELIVERED: 'delivered' },
}

const nodes: { id: StateId; x: number; y: number; rows: string[]; initial?: boolean }[] = [
  { id: 'pending', x: 0, y: 24, rows: ['PAY', 'CANCEL'], initial: true },
  { id: 'paid', x: 380, y: 43, rows: ['SHIP'] },
  { id: 'shipped', x: 760, y: 62, rows: ['DELIVERED'] },
]

const finals: { id: StateId; cx: number; cy: number }[] = [
  { id: 'delivered', cx: 1111, cy: 120 },
  { id: 'cancelled', cx: 395, cy: 250 },
]

const edges: { from: StateId; event: string; d: string; head: string }[] = [
  {
    from: 'pending',
    event: 'CANCEL',
    d: 'M281 118 H322 Q330 118 330 126 V242 Q330 250 338 250 H372',
    head: 'M366 245 L374 250 L366 255',
  },
  { from: 'shipped', event: 'DELIVERED', d: 'M1041 120 H1090', head: 'M1084 115 L1092 120 L1084 125' },
  { from: 'paid', event: 'SHIP', d: 'M661 101 H754', head: 'M748 96 L756 101 L748 106' },
  { from: 'pending', event: 'PAY', d: 'M281 82 H374', head: 'M368 77 L376 82 L368 87' },
]

const state = ref<StateId>('paid')
const sent = ref<string[]>(['PAY'])

const done = computed(() => !transitions[state.value])

function send(from: StateId, event: string) {
  const next = from === state.value ? transitions[from]?.[event] : undefined
  if (!next) return
  state.value = next
  sent.value = [...sent.value, event]
}

function reset() {
  state.value = 'pending'
  sent.value = []
}

function edgeClass(e: { from: StateId; event: string }) {
  if (sent.value.includes(e.event)) return 'taken'
  return e.from === state.value ? 'open' : 'idle'
}

function nameX(n: { x: number; initial?: boolean; id: StateId }) {
  return n.x + 14 + (n.initial ? 24 : 0) + (n.id === state.value ? 16 : 0)
}
</script>

<template>
  <section class="band">
    <div class="fate-wrap inner">
      <div class="top">
        <span class="machine">machine order</span>
        <ul class="legend">
          <li><i class="k taken" />taken</li>
          <li><i class="k open" />can take now</li>
          <li><i class="k idle" />drawn, not taken</li>
        </ul>
      </div>

      <div class="scroll">
        <svg viewBox="0 0 1200 290" role="group" aria-label="An order statechart you can step through">
          <g v-for="e in edges" :key="e.event" class="edge" :class="edgeClass(e)">
            <path class="line" :d="e.d" />
            <path :d="e.head" />
          </g>

          <g v-for="n in nodes" :key="n.id" class="node" :class="{ leaf: n.id === state }">
            <rect class="box" :x="n.x + 1.2" :y="n.y + 1.2" width="277.6" :height="37.6 + n.rows.length * 36" rx="11" />
            <path
              class="head"
              :d="`M${n.x + 1.2} ${n.y + 40} V${n.y + 12} Q${n.x + 1.2} ${n.y + 1.2} ${n.x + 12} ${n.y + 1.2} H${n.x + 268} Q${n.x + 278.8} ${n.y + 1.2} ${n.x + 278.8} ${n.y + 12} V${n.y + 40} Z`"
            />
            <g v-if="n.initial" class="initial">
              <circle :cx="n.x + 18" :cy="n.y + 20.5" r="4.5" />
              <path :d="`M${n.x + 22} ${n.y + 20.5} H${n.x + 30}`" />
            </g>
            <path
              v-if="n.id === state"
              class="play"
              :d="`M${nameX(n) - 14} ${n.y + 15.5} v10 l8 -5 Z`"
            />
            <text class="name" :x="nameX(n)" :y="n.y + 25">{{ n.id }}</text>

            <g
              v-for="(row, i) in n.rows"
              :key="row"
              class="row"
              :class="{ live: n.id === state }"
              :role="n.id === state ? 'button' : undefined"
              :tabindex="n.id === state ? 0 : undefined"
              :aria-label="n.id === state ? `Send ${row}` : undefined"
              @click="send(n.id, row)"
              @keydown.enter.prevent="send(n.id, row)"
              @keydown.space.prevent="send(n.id, row)"
            >
              <path class="rule" :d="`M${n.x + 2.4} ${n.y + 40 + i * 36} H${n.x + 277.6}`" />
              <rect class="hit" :x="n.x + 6" :y="n.y + 44 + i * 36" width="268" height="28" rx="6" />
              <text class="event" :x="n.x + 14" :y="n.y + 62 + i * 36">{{ row }}</text>
            </g>
          </g>

          <g v-for="f in finals" :key="f.id" class="final" :class="{ on: f.id === state }">
            <circle class="ring" :cx="f.cx" :cy="f.cy" r="12" />
            <circle class="core" :cx="f.cx" :cy="f.cy" r="5" />
            <text class="name" :x="f.cx + 22" :y="f.cy + 4.5">{{ f.id }}</text>
          </g>
        </svg>
      </div>

      <div class="status">
        <div class="pair">
          <span class="fate-label">Active state</span>
          <span class="fate-path" aria-live="polite">order.{{ state }}</span>
        </div>
        <div class="pair">
          <span class="fate-label">Events sent</span>
          <span v-for="(ev, i) in sent" :key="i" class="fate-chip">{{ ev }}</span>
          <span v-if="!sent.length" class="none">none</span>
        </div>
        <button class="reset" type="button" @click="reset">reset</button>
        <span class="hint">{{ done ? 'Final state. Nothing left to send.' : 'Click an event in the lime state to send it.' }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.band {
  border-top: 1px solid var(--fate-line);
  border-bottom: 1px solid var(--fate-line);
  background-image: radial-gradient(var(--fate-line) 1.5px, transparent 1.5px);
  background-size: 24px 24px;
}

.inner {
  display: flex;
  flex-direction: column;
  gap: 28px;
  padding-top: 40px;
  padding-bottom: 32px;
}

.top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px 32px;
}

.machine {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--fate-muted);
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  font-size: 12px;
  color: var(--fate-muted);
}

.legend li {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.k {
  width: 28px;
  border-top: 2.4px solid var(--fate-edge);
}

.k.taken {
  border-color: var(--fate-active-line);
}

.k.open {
  border-top-style: dashed;
  border-color: var(--fate-active-line);
}

.scroll {
  overflow-x: auto;
}

svg {
  display: block;
  width: 100%;
  min-width: 760px;
  height: auto;
}

.edge {
  fill: none;
  stroke: var(--fate-edge);
  stroke-width: 2.4;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.edge.open,
.edge.taken {
  stroke: var(--fate-active-line);
}

.edge.open .line {
  stroke-dasharray: 7 7;
}

.edge.taken .line {
  stroke-dasharray: 400;
}

.box {
  fill: var(--fate-panel);
  stroke: var(--fate-edge);
  stroke-width: 2.4;
}

.head {
  fill: var(--fate-panel-2);
}

.rule {
  stroke: var(--fate-line);
  stroke-width: 1;
}

.name {
  font-family: var(--vp-font-family-base);
  font-size: 13px;
  font-weight: 600;
  fill: var(--fate-fg);
}

.event {
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
  fill: var(--fate-fg);
}

.initial circle {
  fill: var(--fate-active-line);
}

.initial path {
  stroke: var(--fate-active-line);
  stroke-width: 2.4;
  stroke-linecap: round;
}

.hit {
  fill: transparent;
}

.row {
  outline: none;
}

.row.live {
  cursor: pointer;
}

.row.live:hover .hit {
  fill: color-mix(in srgb, var(--fate-active-fg) 12%, transparent);
}

.row.live:focus-visible .hit {
  stroke: var(--fate-active-fg);
  stroke-width: 2;
}

.leaf .box {
  fill: var(--fate-active);
  stroke: var(--fate-active-line);
}

.leaf .head {
  fill: none;
}

.leaf .rule {
  stroke: color-mix(in srgb, var(--fate-active-fg) 22%, transparent);
}

.leaf .name,
.leaf .event,
.leaf .play,
.leaf .initial circle {
  fill: var(--fate-active-fg);
}

.leaf .initial path {
  stroke: var(--fate-active-fg);
}

.ring {
  fill: none;
  stroke: var(--fate-edge);
  stroke-width: 2.4;
}

.core {
  fill: var(--fate-edge);
}

.final.on .ring {
  stroke: var(--fate-active-line);
}

.final.on .core,
.final.on .name {
  fill: var(--fate-active-line);
}

.status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 32px;
  border-top: 1px solid var(--fate-line);
  padding-top: 16px;
  min-height: 61px;
}

.pair {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.none,
.hint {
  font-size: 12px;
  color: var(--fate-muted);
}

.hint {
  margin-left: auto;
}

.reset {
  border: 1.5px solid var(--fate-line);
  border-radius: 8px;
  padding: 0 12px;
  height: 32px;
  font-size: 13px;
  font-weight: 500;
  color: var(--fate-fg);
  background: var(--fate-panel);
}

.reset:hover {
  background: var(--fate-panel-2);
}

.reset:focus-visible {
  outline: 2px solid var(--fate-accent);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: no-preference) {
  .edge.taken .line {
    animation: draw 0.6s ease-out both;
  }
}

@keyframes draw {
  from {
    stroke-dashoffset: 400;
  }
  to {
    stroke-dashoffset: 0;
  }
}
</style>
