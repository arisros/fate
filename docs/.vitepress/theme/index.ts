import { h } from 'vue'
import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme-without-fonts'
import '@fontsource-variable/rubik/wght.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/500.css'
import FateHome from './components/FateHome.vue'
import NavWordmark from './components/NavWordmark.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'nav-bar-title-after': () => h(NavWordmark),
      'home-hero-before': () => h(FateHome),
    }),
} satisfies Theme
