// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Akita',
  tagline: 'A discrete-event simulation framework for computer architecture',
  favicon: 'img/favicon.ico',


  url: 'https://sarchlab.github.io',
  baseUrl: '/',

  organizationName: 'sarchlab',
  projectName: 'akita',

  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    mermaid: true,
  },
  themes: ['@docusaurus/theme-mermaid'],

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: '../doc/core',
          routeBasePath: '/',
          sidebarPath: './sidebars-core.js',
          editUrl: 'https://github.com/sarchlab/akita/blob/main/doc/core/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      /** @type {import('@docusaurus/plugin-content-docs').Options} */
      ({
        id: 'tutorial',
        path: '../doc/tutorial',
        routeBasePath: 'tutorial',
        sidebarPath: './sidebars-tutorial.js',
        editUrl: 'https://github.com/sarchlab/akita/blob/main/doc/tutorial/',
      }),
    ],
    [
      '@docusaurus/plugin-content-docs',
      /** @type {import('@docusaurus/plugin-content-docs').Options} */
      ({
        id: 'daisen',
        path: '../daisen2',
        routeBasePath: 'tools/daisen',
        sidebarPath: './sidebars-tools.js',
        editUrl: 'https://github.com/sarchlab/akita/blob/main/daisen2/',
        include: ['README.md'],
      }),
    ],
    [
      '@docusaurus/plugin-content-docs',
      /** @type {import('@docusaurus/plugin-content-docs').Options} */
      ({
        id: 'akita-rtm',
        path: '../monitoring2',
        routeBasePath: 'tools/akita-rtm',
        sidebarPath: './sidebars-tools.js',
        editUrl: 'https://github.com/sarchlab/akita/blob/main/monitoring2/',
        include: ['README.md'],
      }),
    ],
    ...['modeling', 'queueing', 'datarecording', 'tracing', 'simulation', 'examples', 'noc', 'mem'].map(pkg => [
      '@docusaurus/plugin-content-docs',
      /** @type {import('@docusaurus/plugin-content-docs').Options} */
      ({
        id: `pkg-${pkg}`,
        path: `../${pkg}`,
        routeBasePath: `packages/${pkg}`,
        sidebarPath: `./sidebars/${pkg}.js`,
        editUrl: `https://github.com/sarchlab/akita/blob/main/${pkg}/`,
        include: ['**/README.md'],
        exclude: ['**/node_modules/**', '**/static/**'],
      }),
    ]),
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: 'Akita',
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'tutorialSidebar',
            docsPluginId: 'tutorial',
            position: 'left',
            label: 'Tutorial',
          },
          {
            type: 'docSidebar',
            sidebarId: 'coreGroupSidebar',
            docsPluginId: 'pkg-modeling',
            position: 'left',
            label: 'Core',
          },
          {
            type: 'docSidebar',
            sidebarId: 'componentsGroupSidebar',
            docsPluginId: 'pkg-noc',
            position: 'left',
            label: 'First-party Components',
          },
          {
            type: 'dropdown',
            label: 'Tools',
            position: 'left',
            items: [
              {
                type: 'docSidebar',
                sidebarId: 'toolsSidebar',
                docsPluginId: 'daisen',
                label: 'Daisen',
              },
              {
                type: 'docSidebar',
                sidebarId: 'toolsSidebar',
                docsPluginId: 'akita-rtm',
                label: 'Akita RTM',
              },
            ],
          },
          {
            href: 'https://github.com/sarchlab/akita',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        copyright: `Copyright © ${new Date().getFullYear()} <a href="https://sarchlab.org">SarchLab</a>. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['go'],
      },
    }),
};

export default config;
