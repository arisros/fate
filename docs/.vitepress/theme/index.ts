import { h } from 'vue'
import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme-without-fonts'
import '@fontsource-variable/rubik/wght.css'
import '@fontsource-variable/jetbrains-mono/wght.css'
import HeroChart from './components/HeroChart.vue'
import HeroInstall from './components/HeroInstall.vue'
import HomeFeatures from './components/HomeFeatures.vue'
import NavWordmark from './components/NavWordmark.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'nav-bar-title-after': () => h(NavWordmark),
      'home-hero-image': () => h(HeroChart),
      'home-hero-actions-after': () => h(HeroInstall),
      'home-features-after': () => h(HomeFeatures),
    }),
} satisfies Theme
