import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitepress'

const manifest = new URL('../../.release-please-manifest.json', import.meta.url)
const version: string = JSON.parse(readFileSync(fileURLToPath(manifest), 'utf8'))['.']

const repo = 'https://github.com/arisros/fate'

export default defineConfig({
  title: 'fate',
  description:
    'A statechart engine for Go. Hierarchical states, parallel regions, and history, with no dependencies beyond the standard library.',
  lang: 'en-US',
  cleanUrls: true,
  appearance: 'dark',

  srcExclude: ['README.md', 'adr/**'],

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#0b1220' }],
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
    logo: { light: '/logo-light.svg', dark: '/logo-dark.svg', alt: '' },
    siteTitle: false,

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
    ],

    socialLinks: [{ icon: 'github', link: repo }],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    editLink: {
      pattern: `${repo}/edit/main/docs/:path`,
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: `Released under the MIT License · v${version} · <a href="https://pkg.go.dev/github.com/arisros/fate/engine">pkg.go.dev</a>`,
      copyright: 'Copyright © Aris Jirat Kurniawan',
    },
  },
})
