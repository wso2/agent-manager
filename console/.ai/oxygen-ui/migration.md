# Migration Guide to Oxygen UI

## Table of Contents
- [Overview](#overview)
- [Install Packages](#install-packages)
- [Update Imports](#update-imports)
- [ESLint Plugin Setup](#eslint-plugin-setup)
- [Common Patterns](#common-patterns)

---

## Overview

Migrating to Oxygen UI involves:
1. Installing Oxygen UI packages
2. Replacing direct `@mui/*` imports with `@wso2/oxygen-ui`
3. Replacing `lucide-react` imports with `@wso2/oxygen-ui-icons-react`
4. Wrapping app with `OxygenUIThemeProvider`
5. Setting up ESLint plugin to prevent regressions

---

## Install Packages

```bash
# Add Oxygen UI packages
pnpm add @wso2/oxygen-ui @wso2/oxygen-ui-icons-react

# Add ESLint plugin (dev dependency)
pnpm add -D @wso2/eslint-plugin-oxygen-ui

# Ensure peer dependencies
pnpm add @emotion/react @emotion/styled date-fns
```

---

## Update Imports

### MUI Components

**Before:**
```tsx
import { Box, Stack, Button } from '@mui/material';
import Typography from '@mui/material/Typography';
```

**After:**
```tsx
import { Box, Stack, Button, Typography } from '@wso2/oxygen-ui';
```

### MUI X Components

**Before:**
```tsx
import { DataGrid } from '@mui/x-data-grid';
import { DatePicker } from '@mui/x-date-pickers';
import { BarChart } from '@mui/x-charts';
```

**After:**
```tsx
import { DataGrid, DatePickers, Charts } from '@wso2/oxygen-ui';

const { DataGrid: DataGridComponent } = DataGrid;
const { DatePicker } = DatePickers;
const { BarChart } = Charts;
```

### Icons

**Before:**
```tsx
import { Settings, Bell, User } from 'lucide-react';
```

**After:**
```tsx
import { Settings, Bell, User } from '@wso2/oxygen-ui-icons-react';
```

### Theme Provider

**Before:**
```tsx
import { ThemeProvider, createTheme } from '@mui/material';

const theme = createTheme({ ... });

<ThemeProvider theme={theme}>
  <App />
</ThemeProvider>
```

**After:**
```tsx
import { OxygenUIThemeProvider } from '@wso2/oxygen-ui';

<OxygenUIThemeProvider>
  <App />
</OxygenUIThemeProvider>
```

---

## ESLint Plugin Setup

### ESLint 9+ (Flat Config)

```js
// eslint.config.js
import oxygenUIPlugin from '@wso2/eslint-plugin-oxygen-ui';

export default [
  oxygenUIPlugin.configs.recommended,
  // ... other configs
];
```

### Custom Configuration

```js
// eslint.config.js
import oxygenUIPlugin from '@wso2/eslint-plugin-oxygen-ui';

export default [
  {
    plugins: {
      '@wso2/oxygen-ui': oxygenUIPlugin,
    },
    rules: {
      '@wso2/oxygen-ui/no-direct-mui-imports': ['error', {
        suggestedPackage: '@wso2/oxygen-ui',
        allowedPackages: [], // Temporarily allow specific MUI packages during migration
      }],
      '@wso2/oxygen-ui/no-direct-lucide-imports': ['error', {
        suggestedPackage: '@wso2/oxygen-ui-icons-react',
      }],
    },
  },
];
```

### Auto-fix Imports

```bash
# Find violations
pnpm eslint src/

# Auto-fix imports
pnpm eslint src/ --fix
```

### Gradual Migration

During migration, temporarily allow specific MUI packages:

```js
'@wso2/oxygen-ui/no-direct-mui-imports': ['warn', {
  allowedPackages: ['@mui/x-data-grid', '@mui/x-date-pickers'],
}],
```

---

## Common Patterns

### Layout Migration

**Before (custom layout):**
```tsx
<Box sx={{ display: 'flex' }}>
  <AppBar>...</AppBar>
  <Drawer>...</Drawer>
  <Box component="main">...</Box>
</Box>
```

**After (Oxygen AppShell):**
```tsx
import { AppShell, Header, Sidebar } from '@wso2/oxygen-ui';

<AppShell>
  <AppShell.Navbar>
    <Header>...</Header>
  </AppShell.Navbar>
  <AppShell.Sidebar>
    <Sidebar>...</Sidebar>
  </AppShell.Sidebar>
  <AppShell.Main>...</AppShell.Main>
</AppShell>
```

### Theme Customization Migration

**Before:**
```tsx
const theme = createTheme({
  palette: {
    primary: { main: '#1976d2' },
  },
});
```

**After:**
```tsx
import { extendTheme } from '@mui/material/styles';
import { OxygenThemeBase, OxygenUIThemeProvider } from '@wso2/oxygen-ui';

const customTheme = extendTheme({
  ...OxygenThemeBase,
  colorSchemes: {
    light: {
      palette: { primary: { main: '#1976d2' } },
    },
  },
});

<OxygenUIThemeProvider theme={customTheme}>
  <App />
</OxygenUIThemeProvider>
```

### Icon Button Migration

**Before:**
```tsx
import IconButton from '@mui/material/IconButton';
import SettingsIcon from '@mui/icons-material/Settings';

<IconButton><SettingsIcon /></IconButton>
```

**After:**
```tsx
import { IconButton } from '@wso2/oxygen-ui';
import { Settings } from '@wso2/oxygen-ui-icons-react';

<IconButton><Settings size={20} /></IconButton>
```

### Color Mode Migration

**Before:**
```tsx
import { useColorScheme } from '@mui/material';

const { mode, setMode } = useColorScheme();
```

**After:**
```tsx
import { ColorSchemeToggle, useThemeContent } from '@wso2/oxygen-ui';

// Simple toggle
<ColorSchemeToggle />

// Conditional content
const content = useThemeContent({
  light: 'Light content',
  dark: 'Dark content',
});
```

---

## Migration Checklist

- [ ] Install `@wso2/oxygen-ui` and `@wso2/oxygen-ui-icons-react`
- [ ] Install peer dependencies (`@emotion/react`, `@emotion/styled`, `date-fns`)
- [ ] Wrap root component with `OxygenUIThemeProvider`
- [ ] Replace `@mui/material` imports with `@wso2/oxygen-ui`
- [ ] Replace `@mui/icons-material` or `lucide-react` with `@wso2/oxygen-ui-icons-react`
- [ ] Update MUI X imports to use namespaced exports
- [ ] Install and configure `@wso2/eslint-plugin-oxygen-ui`
- [ ] Run ESLint with `--fix` to auto-correct remaining imports
- [ ] Test light/dark mode functionality
- [ ] Verify theme styling matches expectations
