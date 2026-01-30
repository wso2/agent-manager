import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'WSO2 Agent Manager',
  tagline: 'Build, Deploy, and Manage AI Agents at Scale',
  favicon: 'img/favicon.ico',

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Set the production url of your site here
  url: 'https://wso2.github.io',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/agent-manager/',

  // Set true for GitHub pages deployment.
  trailingSlash: true,

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'wso2', // Usually your GitHub org/user name.
  projectName: 'agent-manager', // Usually your repo name.

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  // Enable mermaid for markdown files
  markdown: {
    mermaid: true,
  },

  // Enable mermaid theme
  themes: ['@docusaurus/theme-mermaid'],

  presets: [
    [
      'classic',
      {
        docs: {
          lastVersion: 'v0.3.0',
          versions: {
            current: {
              label: 'Next',
              banner: 'unreleased',
            },
          },
          sidebarPath: './sidebars.ts',
          showLastUpdateAuthor: true,
          showLastUpdateTime: true,
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/wso2/agent-manager/edit/main/website/',
        },
        blog: false, // Disable blog until we have content
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Replace with your project's social card
    // image: 'img/amp-social-card.png',
    // Algolia search configuration
    // To enable search, apply for Algolia DocSearch at https://docsearch.algolia.com/apply/
    // Once approved, you'll receive your appId and apiKey
    // algolia: {
    //   appId: 'YOUR_APP_ID',
    //   apiKey: 'YOUR_SEARCH_API_KEY',
    //   indexName: 'agent-manager',
    //   contextualSearch: true,
    //   searchParameters: {},
    // },
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'WSO2 Agent Manager',
      // logo: {
      //   alt: 'AMP Logo',
      //   src: 'img/logo.svg',
      //   srcDark: 'img/logo-dark.svg',
      // },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Documentation',
        },
        {
          type: 'docsVersionDropdown',
          position: 'right',
          dropdownActiveClassDisabled: true,
        },
        {
          href: 'https://github.com/wso2/agent-manager',
          position: 'right',
          className: 'header-github-link',
          'aria-label': 'GitHub repository',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Overview',
              to: '/docs/overview/what-is-amp',
            },
            {
              label: 'Quick Start',
              to: '/docs/getting-started/quick-start',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'GitHub Discussions',
              href: 'https://github.com/wso2/agent-manager/discussions',
            },
            {
              label: 'Issues',
              href: 'https://github.com/wso2/agent-manager/issues',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/wso2/agent-manager',
            },
            {
              label: 'WSO2',
              href: 'https://wso2.com',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} WSO2 LLC. Licensed under Apache License 2.0.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
