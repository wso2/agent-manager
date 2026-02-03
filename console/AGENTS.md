# Oxygen UI - AI Development Guide

> WSO2's React component library built on Material-UI v7

## Quick Start

```tsx
import { OxygenUIThemeProvider, OxygenTheme, Button } from '@wso2/oxygen-ui';

function App() {
  return (
    <OxygenUIThemeProvider theme={OxygenTheme}>
      <Button variant="contained">Click me</Button>
    </OxygenUIThemeProvider>
  );
}
```

## Critical Rules

1. **Import from `@wso2/oxygen-ui`** - Never import directly from `@mui/material`
2. **Always wrap with `OxygenUIThemeProvider`** - Required at app root
3. **Icons from `@wso2/oxygen-ui-icons-react`** - Separate package for icons
4. **MUI X uses namespaces** - `DataGrid.DataGrid`, `Charts.LineChart`, `DatePickers.DatePicker`
5. **Use theme tokens** - Use `sx` prop with theme values, not hardcoded colors/spacing

## Import Patterns

```tsx
// Components - import from @wso2/oxygen-ui
import { Button, TextField, Box, Typography, ListingTable } from '@wso2/oxygen-ui';

// Icons - import from @wso2/oxygen-ui-icons-react
import { HomeIcon, SettingsIcon, UserIcon } from '@wso2/oxygen-ui-icons-react';

// MUI X - use as namespaces
import { DataGrid, Charts, DatePickers, TreeView } from '@wso2/oxygen-ui';
<DataGrid.DataGrid rows={rows} columns={columns} />
<Charts.LineChart series={series} />
```

## Theme Tokens (Always Use)

```tsx
// CORRECT - Use theme tokens
<Box sx={{ p: 2, bgcolor: 'background.paper', color: 'text.primary' }} />

// WRONG - Never hardcode
<Box sx={{ padding: '16px', backgroundColor: '#ffffff', color: '#333' }} />
```

## Available Themes

- `OxygenTheme` - Default theme (recommended)
- `AcrylicOrangeTheme` - Orange accent variant
- `AcrylicPurpleTheme` - Purple accent variant
- `ClassicTheme` - Traditional styling
- `HighContrastTheme` - Accessibility-focused

## Custom Oxygen Components

- **Layout**: `AppShell`, `Header`, `Sidebar`, `Footer`
- **Data Display**: `ListingTable`, `PageTitle`, `CodeBlock`
- **Forms**: `Form.TextInput`, `Form.SelectInput`, `Form.Wizard`
- **Feedback**: `NotificationPanel`, `NotificationBanner`
- **Theming**: `OxygenUIThemeProvider`, `ThemeSwitcher`, `ColorSchemeToggle`

## Detailed Documentation

For comprehensive API reference and examples, see:
- [Component API Reference](./.ai/oxygen-ui/components.md)
- [UI Patterns & Examples](./.ai/oxygen-ui/patterns.md)
- [Theme Customization](./.ai/oxygen-ui/theming.md)
- [Migration Guide](./.ai/oxygen-ui/migration.md)

---

For Claude Code users with enhanced features (invokable skills): `npx @wso2/oxygen-ui init --claude`
