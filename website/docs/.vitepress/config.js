export default {
  ignoreDeadLinks: process.env.NODE_ENV === 'development',
  title: "Atun",
  description: "AWS Tagged Tunnel - Secure tunneling made simple",
  themeConfig: {
    logo: '/logo.png',
    nav: [
      { text: 'Guide', link: '/guide/' },
      // { text: 'Reference', link: '/reference/' },
      { text: 'GitHub', link: 'https://github.com/automationd/atun' }
    ],
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
      copyright: '© 2025 Atun Contributors'
    }
  }
}
