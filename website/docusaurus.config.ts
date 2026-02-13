import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import fs from 'fs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)
const versions: string[] = JSON.parse(fs.readFileSync('./versions.json', 'utf-8'));
const latestVersion = versions[0]; // First entry is always the latest

const config: Config = {
  title: 'WSO2 Agent Manager',
  tagline: 'Build, Deploy, and Manage AI Agents at Scale',
  favicon: 'img/WSO2-Logo.png',

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
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  // Enable mermaid theme
  themes: ['@docusaurus/theme-mermaid'],
  plugins: [
    '@signalwire/docusaurus-plugin-llms-txt'],

  presets: [
    [
      'classic',
      {
        docs: {
          lastVersion: latestVersion,
          versions: {
            current: {
              label: 'Next',
              banner: 'unreleased',
            },
            [latestVersion]: {
              label: latestVersion,
              path: latestVersion,
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
    announcementBar: {
      id: `release_${latestVersion.replace(/\./g, '_')}`,
      content:
        `🎉 WSO2 Agent Manager <a target="_blank" rel="noopener noreferrer" href="https://github.com/wso2/agent-manager/releases/tag/amp%2F${latestVersion}">${latestVersion}</a> has been released! Explore what's new. 🎉`,
      isCloseable: true,
    },

    algolia: {
      appId: 'HGUIB02S86',
      apiKey: '5499faf1eb8741fc9f7fcfebe844572e',
      indexName: 'Agent Manager Documentation Site (Docusaurus)',
      contextualSearch: true,
      searchParameters: {},
    },
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      logo: {
        alt: 'WSO2 Agent Manager Logo',
        src: 'img/WSO2 Agent Manager Logo_Black.svg',
        srcDark: 'img/WSO2 Agent Manager Logo_white.svg',
      },
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
              to: `/docs/${latestVersion}/overview/what-is-amp`,
            },
            {
              label: 'Quick Start',
              to: `/docs/${latestVersion}/getting-started/quick-start`,
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
