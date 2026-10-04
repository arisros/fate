<script setup lang="ts">
import { useData, withBase } from 'vitepress'

interface Highlight {
  title: string
  details: string
  link: string
  glyph: 'nested' | 'none' | 'final' | 'effect' | 'clock' | 'diff'
}

const { frontmatter } = useData()
</script>

<template>
  <section v-if="frontmatter.highlights" class="fate-feats">
    <ul class="fate-wrap">
      <li v-for="item in frontmatter.highlights as Highlight[]" :key="item.title">
        <svg viewBox="0 0 32 32" aria-hidden="true">
          <template v-if="item.glyph === 'nested'">
            <rect class="line" x="3" y="3" width="26" height="26" rx="8" />
            <rect class="acc" x="13" y="13" width="11" height="11" rx="3.5" />
          </template>
          <template v-else-if="item.glyph === 'none'">
            <circle class="line" cx="16" cy="16" r="12" />
            <path class="line" d="M8 24 L24 8" />
          </template>
          <template v-else-if="item.glyph === 'final'">
            <circle class="ink" cx="6" cy="16" r="3.4" />
            <path class="line" d="M9 16 H17" />
            <circle class="line" cx="24" cy="16" r="5.5" />
            <circle class="acc" cx="24" cy="16" r="2.2" />
          </template>
          <template v-else-if="item.glyph === 'effect'">
            <rect class="line" x="3" y="8" width="16" height="16" rx="5" />
            <path class="line" d="M19 16 H24" stroke-dasharray="1 4" />
            <rect class="acc" x="24" y="11.5" width="6" height="9" rx="2" />
          </template>
          <template v-else-if="item.glyph === 'clock'">
            <circle class="line" cx="16" cy="16" r="12" />
            <path class="line" d="M16 9 V16 L21 19" />
          </template>
          <template v-else-if="item.glyph === 'diff'">
            <path class="line" d="M4 9 H16 M4 16 H24 M4 23 H12" />
            <rect class="acc" x="20" y="20" width="8" height="6" rx="2" />
          </template>
        </svg>
        <div>
          <h2>
            <a :href="withBase(item.link)">{{ item.title }}</a>
          </h2>
          <p>{{ item.details }}</p>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.fate-feats {
  border-top: 1px solid var(--fate-line);
}

ul {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  gap: 0 48px;
  padding-top: 88px;
  padding-bottom: 88px;
}

li {
  display: flex;
  gap: 16px;
  border-top: 1px solid var(--fate-line);
  padding: 28px 0;
  min-width: 0;
}

svg {
  flex: none;
  margin-top: 2px;
  width: 32px;
  height: 32px;
}

.line {
  fill: none;
  stroke: var(--fate-fg);
  stroke-width: 2.4;
  stroke-linecap: round;
}

.ink {
  fill: var(--fate-fg);
}

.acc {
  fill: var(--fate-active-line);
}

h2 {
  line-height: 1.4;
  font-size: 18px;
  font-weight: 600;
}

h2 a {
  color: var(--fate-fg);
  transition: color 0.25s;
}

h2 a:hover {
  color: var(--fate-accent);
}

p {
  padding-top: 8px;
  line-height: 1.5;
  font-size: 15px;
  color: var(--fate-muted);
}
</style>
