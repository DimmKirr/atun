// import './styles/custom.css'
import {defineConfig, defineConfigWithTheme} from 'vitepress'
// import type { ThemeConfig } from 'vitepress-carbon'
// import baseConfig from 'vitepress-carbon/config'

// https://vitepress.dev/reference/site-config
// export default defineConfigWithTheme<ThemeConfig>({
export default defineConfig({
  // extends: baseConfig,
  title: "Atun — Tunnels on Private Bastions",
  description: "Seamless, IAM-native access to private RDS, Elasticache, DynamoDB, and more. No VPNs, no SSH agents, no friction.",
  srcDir: 'src',
  //base: '/vitepress-carbon-template/', if running on github-pages, set repository name here

  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Examples', link: '/markdown-examples' }
    ],

    search: {
      provider: 'local'
    },

    logo: '/logo.png',

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Introduction', link: '/guide/' },
          { text: 'Quick Start', link: '/guide/quickstart' },
        ]
      },
      {
        text: 'Features',
        items: [
          { text: 'EC2 Router', link: '/guide/ec2-router' },
          { text: 'Tag Schema', link: '/guide/tag-schema' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI Commands', link: '/reference/cli-commands' },
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/automationd/atun' }
    ],
    footer: {
      message: 'Released under Apache 2.0 License.',
      copyright: '©2025 Dmitry Kireev'
    }
  }
})
