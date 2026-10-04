<script setup lang="ts">
import { withBase } from 'vitepress'
import HomeMark from './HomeMark.vue'
</script>

<template>
  <section class="fate-wrap hero">
    <div class="copy">
      <p class="fate-label">A statechart engine for Go</p>
      <h1>Every path is drawn before the first event.</h1>
      <p class="lead">
        fate computes state and never performs side effects. Hierarchical states, parallel regions
        and history, with no dependencies beyond the standard library.
      </p>
      <div class="actions">
        <a class="btn primary" :href="withBase('/guide/getting-started')">Get started</a>
        <a class="btn" :href="withBase('/concepts')">Concepts</a>
      </div>
      <div class="install"><span>$</span>go get github.com/arisros/fate</div>
    </div>
    <div class="mark">
      <HomeMark />
    </div>
  </section>
</template>

<style scoped>
.hero {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 48px 64px;
  padding-top: 64px;
  padding-bottom: 72px;
}

.copy {
  display: flex;
  flex: 1 1 520px;
  flex-direction: column;
  align-items: flex-start;
  gap: 24px;
  min-width: 0;
}

h1 {
  line-height: 1;
  font-size: clamp(40px, 5.2vw, 76px);
  font-weight: 600;
  letter-spacing: -0.03em;
  text-wrap: balance;
}

.lead {
  max-width: 520px;
  line-height: 1.5;
  font-size: 19px;
  color: var(--fate-muted);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.btn {
  display: inline-flex;
  align-items: center;
  border: 1.5px solid var(--fate-line);
  border-radius: 8px;
  padding: 0 20px;
  height: 44px;
  font-size: 15px;
  font-weight: 500;
  color: var(--fate-fg);
  background: var(--fate-panel);
  transition: background-color 0.2s, border-color 0.2s;
}

.btn:hover {
  background: var(--fate-panel-2);
}

.btn.primary {
  border-color: var(--fate-accent);
  font-weight: 600;
  color: var(--fate-accent-fg);
  background: var(--fate-accent);
}

.btn.primary:hover {
  border-color: var(--fate-accent-hover);
  background: var(--fate-accent-hover);
}

.install {
  display: flex;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--fate-line);
  border-radius: 6px;
  padding: 0 16px;
  max-width: 100%;
  height: 44px;
  overflow-x: auto;
  white-space: nowrap;
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  background: var(--fate-bg-2);
}

.install span {
  color: var(--fate-muted);
}

.mark {
  flex: 1 1 360px;
  min-width: 0;
  max-width: 540px;
}

@media (min-width: 960px) {
  .hero {
    padding-top: 80px;
    padding-bottom: 88px;
  }
}
</style>
