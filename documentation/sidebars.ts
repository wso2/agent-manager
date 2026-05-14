import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

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
      label: 'Getting Started',
      collapsed: false,
      items: [
        'getting-started/quick-start',
        {
          type: 'category',
          label: 'Installation',
          collapsed: false,
          items: [
            'getting-started/on-k3d',
            'getting-started/on-your-environment',
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Concepts',
      collapsed: false,
      items: [
        'concepts/observability',
        'concepts/evaluation',
      ],
    },
    {
      type: 'category',
      label: 'Components',
      collapsed: false,
      items: [
        'components/amp-instrumentation',
      ],
    },
    {
      type: 'category',
      label: 'Tutorials',
      collapsed: false,
      items: [
        'tutorials/observe-first-agent',
        'tutorials/evaluation-monitors',
        'tutorials/custom-evaluators',
        'tutorials/register-ai-gateway',
        'tutorials/register-llm-service-provider',
        'tutorials/configure-agent-llm-configuration'
      ],
    },
    {
      type: 'category',
      label: 'Contributing',
      collapsed: false,
      items: [
        'contributing/contributing',
      ],
    },
  ],
};

export default sidebars;
