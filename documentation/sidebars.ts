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
      label: 'Get Started',
      collapsed: false,
      items: [
        'get-started/what-is-amp',
        'get-started/quick-start',
      ],
    },
    {
      type: 'category',
      label: 'Concepts',
      collapsed: false,
      items: [                     
        'concepts/organization',                  
        'concepts/environment', 
        'concepts/project',
        'concepts/agent-lifecycle',    
        'concepts/internal-and-external-agent',
        'concepts/agent-kind-and-catalog',   
        'concepts/observability',
        'concepts/evaluation',
        'concepts/agentid',             
        'concepts/gateway',         
        'concepts/deployment-pipeline',         
        'concepts/agent-sandboxing',                
        'concepts/llm-service-provider',
        'concepts/mcp-proxy',            
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      collapsed: false,
      items: [
        {
          type: 'category',
          label: 'Installation',
          collapsed: false,
          items: [
            'guides/on-k3d',
            'guides/on-your-environment',
            'guides/on-a-vm',
            'guides/cli-installation',
          ],
        },
        {
          type: 'category',
          label: 'Evaluation & Observability',
          collapsed: true,
          items: [
            'guides/custom-evaluators',
            'guides/evaluation-monitors',
            'guides/configure-trace-sampling',
          ],
        },
        {
          type: 'category',
          label: 'Securing Agent Endpoints',
          collapsed: true,
          items: [
            'guides/configure-identity-providers-at-the-gateway',
            {
              type: 'category',
              label: 'Configure Sandboxing',
              collapsed: true,
              items: [
                'guides/isolation-tiers/gvisor',
                'guides/isolation-tiers/kata',
              ],
            },
            'guides/secure-agent-endpoints-with-api-keys',
            'guides/secure-agents-with-jwt-authentication',
            'guides/configure-cors-for-agent-endpoints',
          ],
        },
        {
          type: 'category',
          label: 'Platform Administration',
          collapsed: true,
          items: [
            {
              type: 'doc',
              id: 'guides/register-llm-service-provider',
              label: 'LLM Service Providers',
            },
            'guides/configure-agent-llm-configuration',
            {
              type: 'doc',
              id: 'guides/register-mcp-proxy',
              label: 'MCP Proxies',
            },
            'guides/environment-management',
            'guides/instrumentation-catalog',
            'guides/amp-instrumentation',
          ],
        },
        {
          type: 'category',
          label: 'Agent Identity and Access Management',
          collapsed: true,
          items: [
            'guides/authorize-agent-access-to-mcp-tools',
            'guides/configure-agent-mcp-proxies',
            'guides/use-agentid-in-platform-hosted-agents',
            'guides/retrieve-agentid-for-externally-hosted-agents',
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Tutorials',
      collapsed: false,
      items: [
        'tutorials/create-your-first-agent',
        {
          type: 'doc',
          id: 'tutorials/observe-first-agent',
          label: 'Monitoring an Agent',
        },
      ],
    },
    {
      type: 'category',
      label: 'References',
      collapsed: false,
      items: [
        {
          type: 'category',
          label: 'CLI',
          collapsed: true,
          // Overview is the category's landing page rather than a sibling of the
          // individual command pages: clicking "CLI" opens it.
          link: {type: 'doc', id: 'reference/cli/overview'},
          items: [
            'reference/cli/login',
            'reference/cli/context',
            'reference/cli/project',
            'reference/cli/agent',
            'reference/cli/llm-provider',
            'reference/cli/gateway',
            'reference/cli/skills',
            'reference/cli/version',
            'reference/cli/api',
          ],
        },
        {
          type: 'category',
          label: 'Helm Charts',
          collapsed: true,
          // The category index doubles as the landing page, listing every chart.
          link: {type: 'doc', id: 'reference/helm-charts/index'},
          items: [
            'reference/helm-charts/wso2-agent-manager',
            'reference/helm-charts/wso2-amp-api-platform-gateway-extension',
            'reference/helm-charts/wso2-amp-thunder-extension',
            'reference/helm-charts/wso2-amp-evaluation-extension',
            'reference/helm-charts/wso2-amp-observability-extension',
            'reference/helm-charts/wso2-amp-platform-resources-extension',
          ],
        },
        'reference/mcp-server',
        'reference/observer-mcp-server',
        'reference/authorization',
      ],
    },
    {
      type: 'doc',
      id: 'contributing',
      label: 'Contributing',
    },
  ],
};

export default sidebars;
