<script setup lang="ts">
import { useData } from 'vitepress'

const { frontmatter } = useData()
</script>

<template>
  <div v-if="frontmatter.hero?.install" class="fate-install">{{ frontmatter.hero.install }}</div>
</template>

<style scoped>
.fate-install {
  margin-top: 20px;
  max-width: 100%;
  border: 1px solid var(--vp-c-divider);
  border-radius: 10px;
  padding: 10px 14px;
  overflow-x: auto;
  white-space: nowrap;
  text-align: left;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--vp-code-block-bg);
}

.fate-install::before {
  content: '$ ';
  color: var(--vp-c-brand-1);
}
</style>
