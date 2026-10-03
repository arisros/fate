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
    <ul>
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
  padding: 0 24px;
}

ul {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0 36px;
  margin: 0 auto;
  max-width: 1152px;
}

li {
  display: flex;
  gap: 14px;
  border-top: 1px solid var(--vp-c-divider);
  padding: 18px 0;
  min-width: 0;
}

svg {
  flex: none;
  margin-top: 2px;
  width: 28px;
  height: 28px;
}

.line {
  fill: none;
  stroke: var(--vp-c-text-1);
  stroke-width: 2.4;
  stroke-linecap: round;
}

.ink {
  fill: var(--vp-c-text-1);
}

.acc {
  fill: var(--vp-c-brand-1);
}

h2 {
  line-height: 1.5;
  font-size: 15px;
  font-weight: 600;
}

h2 a {
  color: var(--vp-c-text-1);
  transition: color 0.25s;
}

h2 a:hover {
  color: var(--vp-c-brand-1);
}

p {
  line-height: 1.5;
  font-size: 14px;
  color: var(--vp-c-text-2);
}

@media (min-width: 640px) {
  .fate-feats {
    padding: 0 48px;
  }

  ul {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 960px) {
  .fate-feats {
    padding: 0 64px;
  }

  ul {
    grid-template-columns: repeat(3, 1fr);
  }
}
</style>
