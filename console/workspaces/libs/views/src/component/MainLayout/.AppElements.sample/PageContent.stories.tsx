/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { Meta, StoryObj } from '@storybook/react';
import { PageContent, Card, CardContent, Typography, Box } from '@wso2/oxygen-ui';
import React from 'react';

/**
 * PageContent is a layout component for containing page content with consistent spacing and max-width.
 * It provides automatic centering and responsive padding.
 */
const meta: Meta<typeof PageContent> = {
  title: 'App Elements/Page Content',
  component: PageContent,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component: `
The PageContent component provides a consistent container for page content with automatic centering,
max-width constraints, and theme-aware spacing.

### Features
- **Max Width Control**: Default 1400px, customizable
- **Auto Centering**: Centers content horizontally by default
- **Theme Spacing**: Uses theme.spacing(5) for vertical padding
- **Full Width Option**: Can override max-width for edge-to-edge content
- **Styled Component**: Built with MUI styled API for theme integration

### Usage
\`\`\`tsx
import { PageContent } from '@wso2/oxygen-ui';

// Default usage with centering and max-width
<PageContent>
  <YourContent />
</PageContent>

// Custom max-width
<PageContent maxWidth="1200px">
  <YourContent />
</PageContent>

// Full width
<PageContent fullWidth>
  <YourContent />
</PageContent>

// Disable centering
<PageContent centered={false}>
  <YourContent />
</PageContent>
\`\`\`
        `,
      },
    },
  },
  argTypes: {
    maxWidth: {
      control: 'text',
      description: 'Maximum width of the content container',
      table: {
        defaultValue: { summary: '1400px' },
      },
    },
    centered: {
      control: 'boolean',
      description: 'Whether to center the content horizontally',
      table: {
        defaultValue: { summary: 'true' },
      },
    },
    fullWidth: {
      control: 'boolean',
      description: 'Whether to use full width (ignores maxWidth)',
      table: {
        defaultValue: { summary: 'false' },
      },
    },
    children: {
      control: false,
      description: 'The content to be displayed',
    },
  },
};

export default meta;
type Story = StoryObj<typeof PageContent>;

/**
 * Default usage with centered content and default max-width (1400px).
 */
export const Default: Story = {
  render: () => (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <PageContent>
        <Card>
          <CardContent>
            <Typography variant="h5" gutterBottom>
              Default Page Content
            </Typography>
            <Typography variant="body1" color="text.secondary">
              This content is centered with a maximum width of 1400px and has vertical padding of theme.spacing(5).
              The PageContent component provides consistent spacing and layout for your page content.
            </Typography>
          </CardContent>
        </Card>
      </PageContent>
    </Box>
  ),
};

/**
 * Custom max-width example with a narrower container.
 */
export const CustomMaxWidth: Story = {
  render: () => (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <PageContent maxWidth="800px">
        <Card>
          <CardContent>
            <Typography variant="h5" gutterBottom>
              Custom Max Width
            </Typography>
            <Typography variant="body1" color="text.secondary">
              This content has a custom max-width of 800px, making it ideal for article-style layouts
              or forms where you want to limit the line length for better readability.
            </Typography>
          </CardContent>
        </Card>
      </PageContent>
    </Box>
  ),
};

/**
 * Full width example that stretches edge-to-edge.
 */
export const FullWidth: Story = {
  render: () => (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <PageContent fullWidth>
        <Card>
          <CardContent>
            <Typography variant="h5" gutterBottom>
              Full Width Content
            </Typography>
            <Typography variant="body1" color="text.secondary">
              This content stretches to the full width of the viewport. Perfect for dashboards,
              data tables, or any content that benefits from maximum horizontal space.
            </Typography>
          </CardContent>
        </Card>
      </PageContent>
    </Box>
  ),
};

/**
 * Content without automatic centering.
 */
export const NotCentered: Story = {
  render: () => (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <PageContent centered={false} maxWidth="600px">
        <Card>
          <CardContent>
            <Typography variant="h5" gutterBottom>
              Not Centered
            </Typography>
            <Typography variant="body1" color="text.secondary">
              This content is not centered and aligns to the left with a max-width of 600px.
              Useful for specific layout requirements where left alignment is preferred.
            </Typography>
          </CardContent>
        </Card>
      </PageContent>
    </Box>
  ),
};

/**
 * Multiple content blocks example showing typical page layout.
 */
export const MultipleBlocks: Story = {
  render: () => (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <PageContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          <Card>
            <CardContent>
              <Typography variant="h5" gutterBottom>
                Section 1
              </Typography>
              <Typography variant="body1" color="text.secondary">
                First content block with some information.
              </Typography>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="h5" gutterBottom>
                Section 2
              </Typography>
              <Typography variant="body1" color="text.secondary">
                Second content block with additional details.
              </Typography>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="h5" gutterBottom>
                Section 3
              </Typography>
              <Typography variant="body1" color="text.secondary">
                Third content block demonstrating consistent spacing.
              </Typography>
            </CardContent>
          </Card>
        </Box>
      </PageContent>
    </Box>
  ),
};
