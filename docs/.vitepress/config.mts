import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitepress'

// The released version, read from the file release-please already maintains.
// The previous site hard-coded it and drifted three releases behind; this
// cannot.
const manifest = new URL('../../.release-please-manifest.json', import.meta.url)
const version: string = JSON.parse(readFileSync(fileURLToPath(manifest), 'utf8'))['.']

const repo = 'https://github.com/arisros/fate'

export default defineConfig({
  title: 'fate',
  description:
    'A statechart engine for Go. Hierarchical states, parallel regions, and history, with no dependencies beyond the standard library.',
  lang: 'en-US',
  cleanUrls: true,
  // No lastUpdated: it shells out to git per page, which is not available
  // in the hermetic container build (docs/Dockerfile).

  // README.md files are the GitHub-facing indexes for their directories. The
  // site has its own home page, and adr/README.md is rewritten below.
  srcExclude: ['README.md'],
  rewrites: { 'adr/README.md': 'adr/index.md' },

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#c2ef4e' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'fate — a statechart engine for Go' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Hierarchical states, parallel regions, and history. Deterministic, persistable, and dependency-free.',
      },
    ],
  ],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Guide', link: '/guide/getting-started', activeMatch: '/guide/' },
      { text: 'Concepts', link: '/concepts' },
      { text: 'CLI', link: '/cli' },
      {
        text: `v${version}`,
        items: [
          { text: 'Changelog', link: `${repo}/blob/main/CHANGELOG.md` },
          { text: 'Versioning & deprecation', link: '/versioning' },
          { text: 'Releases', link: `${repo}/releases` },
        ],
      },
      { text: 'Studio', link: 'https://fate-studio.arisjirat.com' },
    ],

    sidebar: [
      {
        text: 'Guide',
        collapsed: false,
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Defining machines', link: '/guide/defining-machines' },
          { text: 'Effects and adapters', link: '/guide/effects-and-adapters' },
          { text: 'Persistence and determinism', link: '/guide/persistence-and-determinism' },
          { text: 'Temporal', link: '/guide/temporal' },
        ],
      },
      {
        text: 'Reference',
        collapsed: false,
        items: [
          { text: 'Concepts', link: '/concepts' },
          { text: 'The fate CLI', link: '/cli' },
          { text: 'Versioning and deprecation', link: '/versioning' },
          { text: 'API reference', link: 'https://pkg.go.dev/github.com/arisros/fate' },
        ],
      },
      {
        text: 'Decisions',
        collapsed: true,
        items: [
          { text: 'Overview', link: '/adr/' },
          { text: '0001 · Provenance and license', link: '/adr/0001-provenance-and-license' },
          { text: '0002 · Public API', link: '/adr/0002-public-api' },
          { text: '0003 · Scheduler and timers', link: '/adr/0003-scheduler-and-timer-model' },
          { text: '0004 · Invoke and spawn', link: '/adr/0004-invoke-spawn-effects' },
          { text: '0005 · Temporal boundary', link: '/adr/0005-temporal-integration-boundary' },
          { text: '0006 · Dropped events', link: '/adr/0006-observability-of-dropped-events' },
        ],
      },
    ],

    socialLinks: [{ icon: 'github', link: repo }],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    editLink: {
      pattern: `${repo}/edit/main/docs/:path`,
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: `Released under the MIT License · v${version} · <a href="https://pkg.go.dev/github.com/arisros/fate">pkg.go.dev</a>`,
      copyright: 'Copyright © Aris Kurniawan',
    },
  },
})
