import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation

 The sidebars can be generated from the filesystem, or explicitly defined here.

 Create as many sidebars as you want.
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [

    {
      type: 'category',
      label: 'Overview',
      collapsed: false,
      items: [
        'overview/what-is-amp',
      ],
    },
    {
      type: 'category',
      label: 'Get Started',
      collapsed: false,
      items: [
        'getting-started/quick-start',
        'getting-started/single-cluster-installation',
      ],
    },
    // More sections will be added as documentation is migrated
  ],
};

export default sidebars;
