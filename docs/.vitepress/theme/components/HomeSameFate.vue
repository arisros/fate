<script setup lang="ts">
import { withBase } from 'vitepress'

const runs = ['Run one', 'Run two, restored from a snapshot']
</script>

<template>
  <section class="same">
    <div class="fate-wrap inner">
      <header>
        <h2 class="fate-h2">Same events, same fate.</h2>
        <p>
          The same machine driven by the same sequence of operations always reaches the same active
          configuration and produces the same persisted bytes.
        </p>
      </header>
      <div class="runs">
        <template v-for="(run, i) in runs" :key="run">
          <span v-if="i" class="eq" aria-label="equals">=</span>
          <div class="run">
            <span class="fate-label">{{ run }}</span>
            <div class="row">
              <span class="fate-chip">PAY</span>
              <span class="fate-chip">SHIP</span>
              <i />
              <span class="fate-path">order.shipped</span>
            </div>
          </div>
        </template>
      </div>
      <a class="fate-more" :href="withBase('/guide/persistence-and-determinism')">
        How persistence and determinism work
      </a>
    </div>
  </section>
</template>

<style scoped>
.same {
  border-top: 1px solid var(--fate-line);
  background: var(--fate-bg-2);
}

.inner {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 40px;
  padding-top: 88px;
  padding-bottom: 72px;
}

header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px 64px;
  width: 100%;
}

header p {
  flex: 1 1 380px;
  max-width: 520px;
  line-height: 1.5;
  font-size: 16px;
  color: var(--fate-muted);
}

.runs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 20px;
  width: 100%;
}

.run {
  display: flex;
  flex: 1 1 320px;
  flex-direction: column;
  gap: 16px;
  border: 1px solid var(--fate-line);
  border-radius: 12px;
  padding: 24px;
  background: var(--fate-panel);
}

.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.row i {
  width: 24px;
  border-top: 2.4px solid var(--fate-edge);
}

.eq {
  font-family: var(--vp-font-family-mono);
  font-size: 40px;
  line-height: 1;
}
</style>
