/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import React, { useState } from 'react'
import type { Meta, StoryObj } from '@storybook/react'
import {
  Button,
  IconButton,
  InputAdornment,
  Box,
  TextField,
  Select,
  MenuItem,
  Autocomplete,
  Form,
} from '@wso2/oxygen-ui'
import { PencilIcon, GitBranchIcon, BoxIcon } from '@wso2/oxygen-ui-icons-react'

/**
 * Individual form elements and components for building custom forms.
 * This collection includes text inputs, selects, autocomplete, typography, stacks, and card buttons.
 *
 * Built using MUI components with Form wrappers for consistent styling.
 */
const meta: Meta = {
  title: 'App Elements/FormElements',
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'A collection of individual form elements and components. \n\n' +
          'Use these building blocks to create custom forms with consistent styling and behavior.',
      },
    },
  },
  tags: ['autodocs'],
}

export default meta
type Story = StoryObj

// ========================================
// Text Input Components
// ========================================

/**
 * TextInput - Basic text input with label
 */
export const TextInputBasic: Story = {
  render: () => {
    const [value, setValue] = useState('')
    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Display Name" name="displayName">
          <TextField
            id="displayName"
            placeholder="Enter display name"
            value={value}
            onChange={e => setValue(e.target.value)}
            fullWidth
          />
        </Form.ElementWrapper>
      </Box>
    )
  },
}

/**
 * TextInput - With icon adornment
 */
export const TextInputWithAdornment: Story = {
  render: () => {
    const [value, setValue] = useState('/')
    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Component Directory" name="componentDirectory">
          <TextField
            id="componentDirectory"
            value={value}
            onChange={e => setValue(e.target.value)}
            slotProps={{
              input: {
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton size="small" edge="end">
                      <PencilIcon size={16} />
                    </IconButton>
                  </InputAdornment>
                ),
              },
            }}
            fullWidth
          />
        </Form.ElementWrapper>
      </Box>
    )
  },
}

/**
 * TextInput - Multiline textarea
 */
export const TextInputMultiline: Story = {
  render: () => {
    const [value, setValue] = useState('')
    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Description" name="description">
          <TextField
            id="description"
            placeholder="Enter description here"
            value={value}
            onChange={e => setValue(e.target.value)}
            multiline
            minRows={3}
            fullWidth
          />
        </Form.ElementWrapper>
      </Box>
    )
  },
}

// ========================================
// Select Input Components
// ========================================

/**
 * SelectInput - Basic select dropdown
 */
export const SelectInputBasic: Story = {
  render: () => {
    const [value, setValue] = useState('wso2')
    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Organization" name="organization">
          <Select
            id="organization"
            value={value}
            onChange={e => setValue(e.target.value as string)}
            fullWidth
          >
            <MenuItem value="wso2">wso2</MenuItem>
            <MenuItem value="asgardeo">asgardeo</MenuItem>
            <MenuItem value="choreo">choreo</MenuItem>
          </Select>
        </Form.ElementWrapper>
      </Box>
    )
  },
}

/**
 * SelectInput - With icon adornment
 */
export const SelectInputWithIcon: Story = {
  render: () => {
    const [value, setValue] = useState('main')
    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Branch" name="branch">
          <Select
            id="branch"
            value={value}
            onChange={e => setValue(e.target.value as string)}
            startAdornment={
              <InputAdornment position="start">
                <GitBranchIcon size={16} />
              </InputAdornment>
            }
            fullWidth
          >
            <MenuItem value="main">main</MenuItem>
            <MenuItem value="develop">develop</MenuItem>
            <MenuItem value="staging">staging</MenuItem>
            <MenuItem value="production">production</MenuItem>
          </Select>
        </Form.ElementWrapper>
      </Box>
    )
  },
}

// ========================================
// Autocomplete Input Components
// ========================================

/**
 * AutocompleteInput - Basic autocomplete with search
 */
export const AutocompleteInputBasic: Story = {
  render: () => {
    const options = ['React', 'Vue', 'Angular', 'Svelte', 'Ember', 'Backbone']
    const [value, setValue] = useState<string | null>(null)

    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Framework" name="framework">
          <Autocomplete
            id="framework"
            options={options}
            value={value}
            onChange={(_, newValue) => setValue(newValue)}
            renderInput={params => <TextField {...params} placeholder="Select a framework" />}
            fullWidth
          />
        </Form.ElementWrapper>
      </Box>
    )
  },
}

/**
 * AutocompleteInput - Multiple selection
 */
export const AutocompleteInputMultiple: Story = {
  render: () => {
    const options = ['Python', 'Java', 'JavaScript', 'Go', 'Rust', 'C++', 'Ruby', 'PHP']
    const [value, setValue] = useState<string[]>([])

    return (
      <Box sx={{ maxWidth: 400 }}>
        <Form.ElementWrapper label="Programming Languages" name="languages">
          <Autocomplete
            id="languages"
            options={options}
            value={value}
            onChange={(_, newValue) => setValue(newValue)}
            multiple
            renderInput={params => <TextField {...params} placeholder="Select languages" />}
            fullWidth
          />
        </Form.ElementWrapper>
      </Box>
    )
  },
}

// ========================================
// Typography Components
// ========================================

/**
 * Typography - Header component (h4 variant)
 */
export const TypographyHeader: Story = {
  render: () => <Form.Header>Create a Service</Form.Header>,
}

/**
 * Typography - Subheader component (h5 variant)
 */
export const TypographySubheader: Story = {
  render: () => <Form.Subheader>Repository Details</Form.Subheader>,
}

/**
 * Typography - Body component (body2 variant)
 */
export const TypographyBody: Story = {
  render: () => <Form.Body>Configure your repository settings for deployment</Form.Body>,
}

// ========================================
// Stack Layout Components
// ========================================

/**
 * Stack - Vertical layout with spacing
 */
export const StackVertical: Story = {
  render: () => (
    <Form.Stack>
      <Form.Header>Title</Form.Header>
      <Form.Subheader>Subtitle</Form.Subheader>
      <Form.Body>This is body text with consistent spacing between elements.</Form.Body>
      <Button variant="contained">Action Button</Button>
    </Form.Stack>
  ),
}

/**
 * Stack - Horizontal layout
 */
export const StackHorizontal: Story = {
  render: () => {
    const [org, setOrg] = useState('wso2')
    const [repo, setRepo] = useState('banking-app')

    return (
      <Form.Stack direction="row" sx={{ maxWidth: 800 }}>
        <Form.ElementWrapper label="Organization" name="organization">
          <Select
            id="organization"
            value={org}
            onChange={e => setOrg(e.target.value as string)}
            fullWidth
          >
            <MenuItem value="wso2">wso2</MenuItem>
            <MenuItem value="asgardeo">asgardeo</MenuItem>
          </Select>
        </Form.ElementWrapper>
        <Form.ElementWrapper label="Repository" name="repository">
          <Select
            id="repository"
            value={repo}
            onChange={e => setRepo(e.target.value as string)}
            fullWidth
          >
            <MenuItem value="banking-app">banking-app</MenuItem>
            <MenuItem value="ecommerce-service">ecommerce-service</MenuItem>
          </Select>
        </Form.ElementWrapper>
      </Form.Stack>
    )
  },
}

// ========================================
// Card Button Components
// ========================================

/**
 * CardButton - Basic selectable card
 */
export const CardButtonBasic: Story = {
  render: () => {
    const [selected, setSelected] = useState(false)

    return (
      <Box sx={{ maxWidth: 200 }}>
        <Form.CardButton onClick={() => setSelected(!selected)} selected={selected}>
          <Form.CardHeader
            title={
              <Form.Stack direction="row" spacing={2} justifyContent="center" alignItems="center">
                <BoxIcon size={32} />
                <Form.Body>Python</Form.Body>
              </Form.Stack>
            }
          />
        </Form.CardButton>
      </Box>
    )
  },
}

/**
 * CardButton - Grid of selectable cards
 */
export const CardButtonGrid: Story = {
  render: () => {
    const [selected, setSelected] = useState<string | null>(null)
    const options = [
      { id: 'python', label: 'Python' },
      { id: 'java', label: 'Java' },
      { id: 'nodejs', label: 'NodeJS' },
      { id: 'go', label: 'Go' },
    ]

    return (
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, maxWidth: 600 }}>
        {options.map(option => (
          <Form.CardButton
            key={option.id}
            sx={{ width: 150 }}
            onClick={() => setSelected(option.id)}
            selected={selected === option.id}
          >
            <Form.CardHeader
              title={
                <Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                  <BoxIcon size={24} />
                  <Form.Body>{option.label}</Form.Body>
                </Form.Stack>
              }
            />
          </Form.CardButton>
        ))}
      </Box>
    )
  },
}
