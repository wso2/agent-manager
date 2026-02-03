# Oxygen UI Theming Guide

## Table of Contents
- [Available Themes](#available-themes)
- [Theme Provider](#theme-provider)
- [Theme Properties](#theme-properties)
- [Dark Mode](#dark-mode)
- [CSS Variables](#css-variables)
- [Custom Themes](#custom-themes)

---

## Available Themes

| Theme | Import | Description |
|-------|--------|-------------|
| OxygenTheme | `OxygenTheme` | Default theme (Acrylic Base) |
| Acrylic Orange | `AcrylicOrangeTheme` | Glass-morphism effects, orange primary |
| Acrylic Purple | `AcrylicPurpleTheme` | Glass-morphism with purple accents |
| Classic | `ClassicTheme` | Standard flat design |
| High Contrast | `HighContrastTheme` | Accessibility-focused with high contrast colors |

```tsx
import {
  OxygenTheme,
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
  ClassicTheme,
  HighContrastTheme
} from '@wso2/oxygen-ui';
```

---

## Theme Provider

### Single Theme (Default)
```tsx
import { OxygenUIThemeProvider } from '@wso2/oxygen-ui';

<OxygenUIThemeProvider>
  <App />
</OxygenUIThemeProvider>
```

### Single Custom Theme
```tsx
import { OxygenUIThemeProvider, ClassicTheme } from '@wso2/oxygen-ui';

<OxygenUIThemeProvider theme={ClassicTheme}>
  <App />
</OxygenUIThemeProvider>
```

### Multiple Themes with Switching
```tsx
import {
  OxygenUIThemeProvider,
  OxygenTheme,
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
  ClassicTheme,
  HighContrastTheme
} from '@wso2/oxygen-ui';

<OxygenUIThemeProvider
  themes={[
    { key: 'default', label: 'Default', theme: OxygenTheme },
    { key: 'orange', label: 'Acrylic Orange', theme: AcrylicOrangeTheme },
    { key: 'purple', label: 'Acrylic Purple', theme: AcrylicPurpleTheme },
    { key: 'classic', label: 'Classic', theme: ClassicTheme },
    { key: 'highContrast', label: 'High Contrast', theme: HighContrastTheme },
  ]}
  initialTheme="default"
>
  <App />
</OxygenUIThemeProvider>
```

### Provider Props
| Prop | Type | Description |
|------|------|-------------|
| `theme` | `Theme` | Single theme object |
| `themes` | `ThemeOption[]` | Array of theme options for switching |
| `initialTheme` | `string` | Initial theme key (when using `themes`) |

---

## Theme Properties

### Color Palette
```tsx
// Primary colors (orange gradient)
theme.palette.primary.main     // #fa7b3f
theme.palette.primary.light
theme.palette.primary.dark

// Secondary, error, warning, info, success
theme.palette.secondary.main
theme.palette.error.main
theme.palette.warning.main
theme.palette.info.main
theme.palette.success.main

// Background
theme.palette.background.default
theme.palette.background.paper
```

### Typography
Font: **Inter Variable** (auto-bundled, no setup required)

```tsx
theme.typography.fontFamily     // 'Inter Variable', sans-serif
theme.typography.fontSize       // 14 (base)

// Heading variants
theme.typography.h1             // 36px, bold
theme.typography.h2             // 30px
theme.typography.h3             // 24px
theme.typography.h4             // 20px
theme.typography.h5             // 16px
theme.typography.h6             // 14px

// Body variants
theme.typography.body1          // 14px
theme.typography.body2          // 12px
theme.typography.caption        // 12px
```

### Custom Oxygen Properties
```tsx
// Glass-morphism blur effects
theme.oxygen.blur.light         // 4px
theme.oxygen.blur.medium        // 8px
theme.oxygen.blur.heavy         // 16px

// Border settings
theme.oxygen.border.width       // '1px'
theme.oxygen.border.style       // 'solid'

// Gradient
theme.oxygen.gradient.primary   // linear-gradient(135deg, #e74420 0%, #fa7b3f 100%)

// Transparent paper (for glass effects)
theme.oxygen.paperTransparent.light  // rgba(255,255,255,0.7)
theme.oxygen.paperTransparent.dark   // rgba(0,0,0,0.5)

// Syntax highlighting colors
theme.oxygen.syntax.light.keyword
theme.oxygen.syntax.dark.keyword
```

---

## Dark Mode

### Color Scheme Toggle
```tsx
import { ColorSchemeToggle } from '@wso2/oxygen-ui';

// Cycles through: light → dark → system
<ColorSchemeToggle />
```

### Adaptive Images
```tsx
import { ColorSchemeImage } from '@wso2/oxygen-ui';

<ColorSchemeImage
  light="/images/logo.svg"
  dark="/images/logo-dark.svg"
  alt="Company Logo"
/>
```

### Programmatic Mode Access
```tsx
import { useTheme } from '@wso2/oxygen-ui';

const theme = useTheme();
const isDark = theme.palette.mode === 'dark';
```

### useThemeContent Hook
```tsx
import { useThemeContent } from '@wso2/oxygen-ui';

const content = useThemeContent({
  light: 'Show this in light mode',
  dark: 'Show this in dark mode'
});
```

---

## CSS Variables

Oxygen UI uses CSS variables with the `--oxygen` prefix.

### Key Variables
```css
/* Colors */
--oxygen-palette-primary-main
--oxygen-palette-background-default
--oxygen-palette-background-paper
--oxygen-palette-text-primary

/* Typography */
--oxygen-typography-fontFamily
--oxygen-typography-fontSize

/* Shadows (disabled by default) */
--oxygen-shadows-0
```

### Accessing in Styles
```tsx
<Box
  sx={{
    backgroundColor: 'var(--oxygen-palette-background-paper)',
    color: 'var(--oxygen-palette-text-primary)',
  }}
/>
```

### Color Scheme Selector
The theme uses `data-color-scheme` attribute for mode switching:

```css
[data-color-scheme="light"] {
  --oxygen-palette-background-default: #ffffff;
}

[data-color-scheme="dark"] {
  --oxygen-palette-background-default: #121212;
}
```

---

## Custom Themes

### Extending Base Theme
```tsx
import { extendTheme } from '@mui/material/styles';
import { OxygenThemeBase } from '@wso2/oxygen-ui';

const customTheme = extendTheme({
  ...OxygenThemeBase,
  colorSchemes: {
    light: {
      palette: {
        primary: {
          main: '#1976d2', // Custom blue
        },
      },
    },
    dark: {
      palette: {
        primary: {
          main: '#90caf9',
        },
      },
    },
  },
});

<OxygenUIThemeProvider theme={customTheme}>
  <App />
</OxygenUIThemeProvider>
```

### Component Overrides
```tsx
const customTheme = extendTheme({
  ...OxygenThemeBase,
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          textTransform: 'none',
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          backdropFilter: 'blur(8px)',
        },
      },
    },
  },
});
```

### Theme Switching in Components
```tsx
import { useThemeSwitcher } from '@wso2/oxygen-ui';

function ThemeSelector() {
  const { currentTheme, setTheme, themes } = useThemeSwitcher();

  return (
    <Select value={currentTheme} onChange={(e) => setTheme(e.target.value)}>
      {themes.map((t) => (
        <MenuItem key={t.key} value={t.key}>
          {t.label}
        </MenuItem>
      ))}
    </Select>
  );
}
```

### LocalStorage Persistence
Theme selection is automatically persisted to localStorage with the key `oxygen-theme`. The provider reads the saved theme on mount and applies it.
