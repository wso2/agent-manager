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

import type { Meta, StoryObj } from '@storybook/react';
import { CodeBlock } from '@wso2/oxygen-ui';
import { Stack, Typography } from '@wso2/oxygen-ui';
import React from 'react';

const meta: Meta<typeof CodeBlock> = {
  title: 'App Elements/CodeBlock',
  component: CodeBlock,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component: 'A code block component with syntax highlighting powered by Prism.js. Supports multiple programming languages and automatically adapts to light/dark themes.',
      },
    },
  },
  argTypes: {
    code: {
      description: 'The code to display',
      control: 'text',
    },
    language: {
      description: 'The programming language for syntax highlighting',
      control: 'select',
      options: ['javascript', 'typescript', 'jsx', 'tsx', 'css', 'bash', 'json', 'markup', 'html'],
    },
    showLineNumbers: {
      description: 'Show line numbers',
      control: 'boolean',
    },
  },
};

export default meta;
type Story = StoryObj<typeof CodeBlock>;

/**
 * TypeScript/TSX code example with React component
 */
export const TypeScript: Story = {
  args: {
    language: 'tsx',
    code: `import { CodeBlock } from '@wso2/oxygen-ui';

function App() {
  return (
    <CodeBlock
      language="tsx"
      code="console.log('Hello, World!');"
    />
  );
}`,
  },
};

/**
 * JavaScript code example
 */
export const JavaScript: Story = {
  args: {
    language: 'javascript',
    code: `function fibonacci(n) {
  if (n <= 1) return n;
  return fibonacci(n - 1) + fibonacci(n - 2);
}

console.log(fibonacci(10)); // 55`,
  },
};

/**
 * CSS code example
 */
export const CSS: Story = {
  args: {
    language: 'css',
    code: `.button {
  background-color: #007bff;
  color: white;
  padding: 10px 20px;
  border-radius: 4px;
  border: none;
  cursor: pointer;
  transition: background-color 0.3s ease;
}

.button:hover {
  background-color: #0056b3;
}`,
  },
};

/**
 * Bash/Shell command example
 */
export const Bash: Story = {
  args: {
    language: 'bash',
    code: `# Install dependencies
npm install @wso2/oxygen-ui

# Start development server
npm run dev

# Build for production
npm run build`,
  },
};

/**
 * JSON data example
 */
export const JSON: Story = {
  args: {
    language: 'json',
    code: `{
  "name": "@wso2/oxygen-ui",
  "version": "1.0.0",
  "description": "A design system built on MUI",
  "dependencies": {
    "@mui/material": "^7.3.4",
    "react": "^19.2.0"
  }
}`,
  },
};

/**
 * HTML markup example
 */
export const HTML: Story = {
  args: {
    language: 'markup',
    code: `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Oxygen UI</title>
</head>
<body>
  <div id="root"></div>
  <script src="main.js"></script>
</body>
</html>`,
  },
};

/**
 * Multiple code blocks showing different languages
 */
export const MultipleLanguages: Story = {
  render: () => (
    <Stack spacing={3}>
      <div>
        <Typography variant="h6" gutterBottom>
          TypeScript Component
        </Typography>
        <CodeBlock
          language="tsx"
          code={`import { OxygenUIThemeProvider, OxygenTheme } from '@wso2/oxygen-ui';

function App() {
  return (
    <OxygenUIThemeProvider themes={[{ key: 'default', theme: OxygenTheme }]}>
      <h1>Hello, Oxygen UI!</h1>
    </OxygenUIThemeProvider>
  );
}`}
        />
      </div>

      <div>
        <Typography variant="h6" gutterBottom>
          Installation Command
        </Typography>
        <CodeBlock
          language="bash"
          code="npm install @wso2/oxygen-ui"
        />
      </div>

      <div>
        <Typography variant="h6" gutterBottom>
          Configuration JSON
        </Typography>
        <CodeBlock
          language="json"
          code={`{
  "theme": "oxygen",
  "darkMode": true,
  "components": ["Button", "Card", "CodeBlock"]
}`}
        />
      </div>
    </Stack>
  ),
};

/**
 * Long code example with scrolling
 */
export const LongCode: Story = {
  args: {
    language: 'tsx',
    code: `import React, { useState, useEffect } from 'react';
import { 
  Box, 
  Button, 
  Card, 
  CardContent, 
  Typography, 
  TextField,
  Stack
} from '@wso2/oxygen-ui';

interface User {
  id: number;
  name: string;
  email: string;
}

const UserList: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');

  useEffect(() => {
    fetchUsers();
  }, []);

  const fetchUsers = async () => {
    try {
      const response = await fetch('/api/users');
      const data = await response.json();
      setUsers(data);
    } catch (error) {
      console.error('Error fetching users:', error);
    } finally {
      setLoading(false);
    }
  };

  const filteredUsers = users.filter(user =>
    user.name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  if (loading) {
    return <Typography>Loading...</Typography>;
  }

  return (
    <Box>
      <TextField
        fullWidth
        label="Search users"
        value={searchTerm}
        onChange={(e) => setSearchTerm(e.target.value)}
        sx={{ mb: 2 }}
      />
      <Stack spacing={2}>
        {filteredUsers.map(user => (
          <Card key={user.id}>
            <CardContent>
              <Typography variant="h6">{user.name}</Typography>
              <Typography color="text.secondary">{user.email}</Typography>
            </CardContent>
          </Card>
        ))}
      </Stack>
    </Box>
  );
};

export default UserList;`,
  },
};
