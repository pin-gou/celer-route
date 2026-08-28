import { themes as prismThemes } from 'prism-react-renderer';

const config = {
  title: 'celer-route',
  tagline: '高性能 AI 网关',
  favicon: 'img/favicon.ico',

  url: 'https://pin-gou.github.io',
  baseUrl: '/celer-route/',
  organizationName: 'pin-gou',
  projectName: 'celer-route',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'zh-CN',
    locales: ['zh-CN', 'en'],
    localeConfigs: {
      'zh-CN': { label: '简体中文' },
      en: { label: 'English' },
    },
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.js',
          routeBasePath: '/',
          showLastUpdateAuthor: true,
          showLastUpdateTime: true,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: 'light',
      disableSwitch: false,
      respectPrefersColorScheme: true,
    },
    image: 'img/social-card.png',
    navbar: {
      title: 'celer-route',
      logo: { alt: 'celer-route', src: 'img/logo.svg' },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: '文档',
        },
        {
          href: 'https://github.com/pin-gou/celer-route',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: '文档',
          items: [
            { label: '入门', to: '/deployment/sqlite' },
            { label: '部署 PostgreSQL', to: '/deployment/postgres' },
          ],
        },
        {
          title: '社区',
          items: [
            { label: 'GitHub', href: 'https://github.com/pin-gou/celer-route' },
            { label: 'Issues', href: 'https://github.com/pin-gou/celer-route/issues' },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} celer-route`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['json', 'bash', 'go', 'yaml'],
    },
  },
};

export default config;