/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import * as React from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { Footer, Box } from '@wso2/oxygen-ui';

/**
 * Footer is a component for displaying application footer with copyright,
 * legal links, and optional version information.
 */
const meta: Meta<typeof Footer> = {
  title: 'App Elements/Footer',
  component: Footer,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component: `
The Footer component displays copyright information and legal links at the bottom of the application.

### Features
- Copyright text with automatic year
- Terms & Conditions link
- Privacy Policy link
- Optional version display
- Custom additional links support
- Responsive layout (stacks on mobile)

### Usage
\`\`\`tsx
import { Footer } from '@wso2/oxygen-ui';

<Footer
  companyName="Acme Corporation"
  termsUrl="/terms"
  privacyUrl="/privacy"
  version="v2.1.0"
  links={[
    { label: 'Documentation', url: '/docs' },
    { label: 'Status', url: '/status' },
  ]}
/>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof Footer>;

/**
 * Basic footer with default settings.
 */
export const Default: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer />
    </Box>
  ),
};

/**
 * Footer with company name.
 */
export const WithCompanyName: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer companyName="Acme Corporation" />
    </Box>
  ),
};

/**
 * Footer with version number.
 */
export const WithVersion: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer
        companyName="Oxygen UI"
        version="v2.1.0"
      />
    </Box>
  ),
};

/**
 * Footer with custom links.
 */
export const WithCustomLinks: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer
        companyName="Acme Corporation"
        termsUrl="/terms"
        privacyUrl="/privacy"
        links={[
          { label: 'Documentation', url: '/docs' },
          { label: 'Status', url: '/status' },
          { label: 'Support', url: '/support' },
        ]}
      />
    </Box>
  ),
};

/**
 * Complete footer with all options.
 */
export const Complete: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer
        companyName="WSO2 LLC"
        copyright="© 2025 WSO2 LLC. All rights reserved."
        termsUrl="https://wso2.com/terms"
        privacyUrl="https://wso2.com/privacy"
        version="v3.0.0-beta.1"
        links={[
          { label: 'Documentation', url: 'https://docs.wso2.com' },
          { label: 'GitHub', url: 'https://github.com/wso2' },
        ]}
      />
    </Box>
  ),
};

/**
 * Footer with custom copyright text.
 */
export const CustomCopyright: Story = {
  render: () => (
    <Box sx={{ minHeight: 200, display: 'flex', flexDirection: 'column' }}>
      <Box sx={{ flex: 1 }} />
      <Footer
        copyright="Made with love by the Oxygen UI team"
        termsUrl="/terms"
        privacyUrl="/privacy"
      />
    </Box>
  ),
};
