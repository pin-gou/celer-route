import { themes as prismThemes } from 'prism-react-renderer';

const config = {
  title: 'pg-gateway',
  tagline: '高性能 AI 网关',
  favicon: 'img/favicon.ico',

  url: 'https://pin-gou.github.io',
  baseUrl: '/pg-gateway/',
  organizationName: 'pin-gou',
  projectName: 'pg-gateway',
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
      title: 'pg-gateway',
      logo: { alt: 'pg-gateway', src: 'img/logo.svg' },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: '文档',
        },
        {
          href: 'https://github.com/pin-gou/pg-gateway',
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
            { label: 'GitHub', href: 'https://github.com/pin-gou/pg-gateway' },
            { label: 'Issues', href: 'https://github.com/pin-gou/pg-gateway/issues' },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} pg-gateway`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['json', 'bash', 'go', 'yaml'],
    },
  },
};

export default config;