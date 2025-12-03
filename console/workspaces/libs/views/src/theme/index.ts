/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { alpha, createTheme, ThemeOptions } from '@mui/material/styles';
// import { theme } from 'src';

// Color palette for AI Agent Management Platform
const colors = {
  // Brand colors
  brand: {
    primary: '#3c3fdd',
    primaryLight: '#d8d9f7',
    primaryDark: '#1e1f66',
    secondary: '#af57db',
    secondaryLight: '#e5d4f5',
    secondaryDark: '#582c6e',
  },

  // Semantic colors - Light mode
  light: {
    // Background colors
    background: {
      default: '#F8F9FB',
      paper: '#FFFFFF',
      elevated: '#FDFDFD',
      overlay: 'rgba(248, 249, 251, 0.95)',
    },
    
    // Text colors
    text: {
      primary: '#212529',
      secondary: '#6C757D',
      tertiary: '#ADB5BD',
      disabled: '#CED4DA',
      heading: '#212529',
      inverse: '#FFFFFF',
    },
    
    // Surface colors
    surface: {
      primary: '#FFFFFF',
      secondary: '#F8F9FB',
      tertiary: '#F1F3F4',
      hover: '#F1F3F4',
      selected: '#E5D4F5',
    },
    
    // Border and divider colors
    border: {
      primary: alpha('#6C757D', 0.2),
      secondary: alpha('#6C757D', 0.1),
      focus: '#3c3fdd',
      error: '#DC3545',
    },
    
    // Status colors
    status: {
      success: '#28A745',
      successLight: '#D4EDDA',
      successDark: '#1E7E34',
      warning: '#FFC107',
      warningLight: '#FFF3CD',
      warningDark: '#E0A800',
      error: '#DC3545',
      errorLight: '#F8D7DA',
      errorDark: '#C82333',
      info: '#17A2B8',
      infoLight: '#D1ECF1',
      infoDark: '#138496',
    },
    
    // Interactive colors
    interactive: {
      primary: '#3c3fdd',
      primaryHover: '#2a2db8',
      secondary: '#af57db',
      secondaryHover: '#9a4bc7',
      disabled: '#CED4DA',
      link: '#3c3fdd',
      linkHover: '#2a2db8',
    },
    
    // Chip colors
    chip: {
      background: '#F8F9FB',
      text: '#6C757D',
      border: alpha('#6C757D', 0.2),
    },
  },

  // Semantic colors - Dark mode
  dark: {
    // Background colors
    background: {
      default: '#0D1117',
      paper: '#161B22',
      elevated: '#21262D',
      overlay: 'rgba(13, 17, 23, 0.95)',
    },
    
    // Text colors
    text: {
      primary: '#F0F6FC',
      secondary: '#8B949E',
      tertiary: '#6E7681',
      disabled: '#484F58',
      heading: '#F0F6FC',
      inverse: '#0D1117',
    },
    
    // Surface colors
    surface: {
      primary: '#161B22',
      secondary: '#21262D',
      tertiary: '#30363D',
      hover: '#21262D',
      selected: '#582C6E',
    },
    
    // Border and divider colors
    border: {
      primary: alpha('#30363D', 0.5),
      secondary: alpha('#30363D', 0.3),
      focus: '#3c3fdd',
      error: '#F85149',
    },
    
    // Status colors
    status: {
      success: '#3FB950',
      successLight: '#1A472A',
      successDark: '#2EA043',
      warning: '#D29922',
      warningLight: '#4D2C00',
      warningDark: '#BF8700',
      error: '#F85149',
      errorLight: '#490202',
      errorDark: '#DA3633',
      info: '#58A6FF',
      infoLight: '#0C2D6B',
      infoDark: '#1F6FEB',
    },
    
    // Interactive colors
    interactive: {
      primary: '#3c3fdd',
      primaryHover: '#2a2db8',
      secondary: '#af57db',
      secondaryHover: '#9a4bc7',
      disabled: '#484F58',
      link: '#58A6FF',
      linkHover: '#1F6FEB',
    },
    
    // Chip colors
    chip: {
      background: '#21262D',
      text: '#8B949E',
      border: alpha('#30363D', 0.5),
    },
  },

  // Shadow definitions
  shadows: {
    light: {
      xs: 'rgba(0, 0, 0, 0.05)',
      sm: 'rgba(0, 0, 0, 0.1)',
      md: 'rgba(0, 0, 0, 0.15)',
      lg: 'rgba(0, 0, 0, 0.2)',
      xl: 'rgba(0, 0, 0, 0.25)',
    },
    dark: {
      xs: 'rgba(0, 0, 0, 0.3)',
      sm: 'rgba(0, 0, 0, 0.4)',
      md: 'rgba(0, 0, 0, 0.5)',
      lg: 'rgba(0, 0, 0, 0.6)',
      xl: 'rgba(0, 0, 0, 0.7)',
    },
  },

  // Legacy color mappings for backward compatibility
  primary: {
    main: '#3c3fdd',
    light: '#d8d9f7',
    dark: '#1e1f66',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#af57db',
    light: '#e5d4f5',
    dark: '#582c6e',
    contrastText: '#ffffff',
  },
  error: {
    main: '#DC3545',
    light: '#E57373',
    dark: '#C82333',
  },
  warning: {
    main: '#FFC107',
    light: '#FFD43B',
    dark: '#E0A800',
  },
  info: {
    main: '#17A2B8',
    light: '#5BC0DE',
    dark: '#138496',
  },
  success: {
    main: '#28A745',
    light: '#5CB85C',
    dark: '#1E7E34',
  },
};

// Custom flat design theme for AI Agent Management Platform
const themeOptions: ThemeOptions = {
  palette: {
    mode: 'light',
    primary: colors.primary,
    secondary: colors.secondary,
    error: colors.error,
    warning: colors.warning,
    info: colors.info,
    success: colors.success,
    background: {
      default: colors.light.background.default,
      paper: colors.light.background.paper,
    },
    text: {
      primary: colors.light.text.primary,
      secondary: colors.light.text.secondary,
    },
    divider: colors.light.border.primary,
  },
  typography: {
    fontFamily: "Poppins, sans-serif",
    h1: {
      fontSize: '2.5rem',
      fontWeight: 600,
      lineHeight: 1.2,
      color: colors.light.text.primary,
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 600,
      lineHeight: 1.3,
      color: colors.light.text.primary,
    },
    h3: {
      fontSize: '1.75rem',
      fontWeight: 600,
      lineHeight: 1.4,
      color: colors.light.text.primary,
    },
    h4: {
      fontSize: '1.5rem',
      fontWeight: 600,
      lineHeight: 1.4,
      color: colors.light.text.primary,
    },
    h5: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.5,
      color: colors.light.text.primary,
    },
    h6: {
      fontSize: '1rem',
      fontWeight: 600,
      lineHeight: 1.6,
      color: colors.light.text.primary,
    },
    body1: {
      fontSize: '1rem',
      lineHeight: 1.5,
    },
    body2: {
      fontSize: '0.875rem',
      lineHeight: 1.43,
      color: 'secondary.main',
    },
    button: {
      fontSize: '0.875rem',
      fontWeight: 500,
      textTransform: 'none',
    },
    caption: {
      fontSize: '0.75rem',
      lineHeight: 1.4,
      color: colors.light.text.secondary,
    },
  },
  shape: {
    borderRadius: 3,
  },
  spacing: 8, // 8px base spacing unit
  shadows: [
    'none',
    `0px 1px 2px ${colors.shadows.light.xs}`,
    `0px 1px 3px ${colors.shadows.light.sm}`,
    `0px 1px 5px ${colors.shadows.light.sm}`,
    `0px 1px 8px ${colors.shadows.light.md}`,
    `0px 1px 10px ${colors.shadows.light.md}`,
    `0px 1px 12px ${colors.shadows.light.md}`,
    `0px 2px 4px ${colors.shadows.light.sm}`,
    `0px 2px 6px ${colors.shadows.light.md}`,
    `0px 2px 8px ${colors.shadows.light.md}`,
    `0px 2px 10px ${colors.shadows.light.lg}`,
    `0px 2px 12px ${colors.shadows.light.lg}`,
    `0px 3px 6px ${colors.shadows.light.md}`,
    `0px 3px 8px ${colors.shadows.light.lg}`,
    `0px 3px 10px ${colors.shadows.light.lg}`,
    `0px 3px 12px ${colors.shadows.light.xl}`,
    `0px 4px 8px ${colors.shadows.light.lg}`,
    `0px 4px 10px ${colors.shadows.light.lg}`,
    `0px 4px 12px ${colors.shadows.light.xl}`,
    `0px 4px 16px ${colors.shadows.light.xl}`,
    `0px 5px 10px ${colors.shadows.light.lg}`,
    `0px 5px 12px ${colors.shadows.light.xl}`,
    `0px 5px 16px ${colors.shadows.light.xl}`,
    `0px 6px 12px ${colors.shadows.light.xl}`,
    `0px 6px 16px ${colors.shadows.light.xl}`,
  ],
  components: {
    MuiCard: {
      styleOverrides: {
        root: {
          boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
          borderRadius: 10,
          backgroundColor: colors.light.background.paper,
          '&:hover': {
            boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
          },
        },
      },
    },
    MuiButton: {
      defaultProps: {
        disableRipple: true,
      },
      styleOverrides: {
        root: {
          borderRadius: 6,
          textTransform: 'none',
          fontWeight: 500,
          minWidth: 'auto',
          // padding: '8px 16px',
          paddingLeft: 16,
          paddingRight: 16,
          '& .MuiSvgIcon-root': {
            fontSize: '1.25rem',
          },
        },
        contained: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;

          if (colorKeySafe === 'primary') {
            const isDark = theme.palette.mode === 'dark';
            return {
              boxShadow: 'none',
              color: theme.palette.primary.contrastText,
              background: `linear-gradient(45deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
              transition: 'opacity 0.3s ease-in-out',
              '&:hover': {
                boxShadow: 'none',
                opacity: 0.9,
              },
              '&:disabled': {
                opacity: 0.6,
                color: alpha(theme.palette.primary.contrastText, 0.6),
                background: isDark
                  ? `linear-gradient(45deg, ${theme.palette.primary.dark}, ${theme.palette.secondary.dark})`
                  : `linear-gradient(45deg, ${theme.palette.primary.light}, ${theme.palette.secondary.light})`,
              },
            };
          }

          return {
            boxShadow: 'none',
            color: palette.contrastText,
            backgroundColor: palette.main,
            transition: 'opacity 0.3s ease-in-out',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: palette.dark || palette.main,
            },
            '&:disabled': {
              opacity: 0.6,
              color: alpha(palette.contrastText, 0.6),
              backgroundColor: alpha(palette.main, 0.4),
            },
          };
        },
        outlined: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;
          return {
            boxShadow: 'none',
            color: palette.main,
            border: `1px solid ${alpha(palette.main, 0.5)}`,
            backgroundColor: 'transparent',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: alpha(palette.main, 0.08),
              border: `1px solid ${palette.main}`,
            },
          };
        },
        text: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;
          
          return {
            boxShadow: 'none',
            color: palette.main,
            backgroundColor: 'transparent',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: alpha(palette.main, 0.08),
            },
          };
        },
      },
    },
    MuiButtonGroup: {
      styleOverrides: {
        root: {
          border: `1px solid ${colors.light.border.primary}`,
          borderRadius: 8,
          '& .MuiButton-root': {
            '&:not(:last-child)': {
              borderRight: `none`,
            },
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          fontWeight: 500,
        },
      },
    },
    MuiIconButton: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiMenuItem: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiButtonBase: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          boxShadow: 'none',
          border: `1px solid ${colors.light.border.primary}`,
          borderRadius: 6,
          padding: 12,
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          marginTop: 24,
          marginBottom: 24,
          backgroundColor: colors.light.background.default,
          padding: 4,
          paddingRight: 8,
          paddingLeft: 8,
          borderRadius: 6,
          '& .MuiInputLabel-root': {
            position: 'absolute',
            transform: 'translateY(-150%)',
            fontWeight: 500,
            fontSize: 14,
            color: colors.light.text.secondary,
          },
          '& .MuiFormHelperText-root': {
            position: 'absolute',
            bottom: 0,
            left: -12,
            transform: 'translateY(150%)',
            fontWeight: 400,
            color: colors.light.text.secondary,
            marginTop: 4,
          },
          transition: 'all 0.2s ease-in-out',
          border: `1px solid ${colors.light.border.primary}`,
          '&:hover': {
            backgroundColor: colors.light.background.paper,
          },
          '&:focus-within': {
            backgroundColor: colors.light.background.paper,
            border: `1px solid ${colors.primary.main}`,
          },  
          '& .MuiInput-underline:before': {
            borderBottom: 'none',
          },
          '& .MuiInput-underline:after': {
            borderBottom: 'none',
          },
          '& .MuiInput-underline:hover:not(.Mui-disabled):before': {
            borderBottom: 'none',
          },
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              border: 'none',
            },
            '&:hover fieldset': {
              border: 'none',
            },
            '&.Mui-focused fieldset': {
              border: 'none',
            },
          },
          '& .MuiInputBase-input': {
            padding: 3,
          },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
          backgroundColor: colors.light.background.paper,
        },
        elevation1: {
          boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
        elevation2: {
          boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
        elevation3: {
          boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: colors.light.background.paper,
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          boxShadow: 'none',
          backgroundColor: colors.light.background.paper,
           borderRight: `1px solid ${colors.light.border.primary}`,
        },
      },
    },
    MuiListItemButton: {
      defaultProps: {
        disableRipple: true,
      },
      styleOverrides: {
        root: {
          borderRadius: 6,
          margin: '4px 8px',
          '&.Mui-selected': {
            backgroundColor: colors.primary.main,
            color: colors.primary.contrastText,
            '&:hover': {
              backgroundColor: colors.primary.dark,
            },
          },
          '&:hover': {
            backgroundColor: colors.light.background.default,
          },
        },
      },
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          backgroundColor: colors.light.background.default,
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          borderBottom: `1px solid ${colors.light.border.primary}`,
          '&:hover': {
            backgroundColor: colors.light.background.default,
          },
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 8px 16px ${colors.shadows.light.lg}`,
          borderRadius: 6,
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 4px 8px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 4px 8px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          boxShadow: `0px 2px 6px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
  },
};

// Create the theme
export const aiAgentTheme = createTheme(themeOptions);

// Create a dark theme variant
export const aiAgentDarkTheme = createTheme({
  ...themeOptions,
  palette: {
    ...themeOptions.palette,
    mode: 'dark',
    primary: colors.primary,
    background: {
      default: colors.dark.background.default,
      paper: colors.dark.background.paper,
    },
    text: {
      primary: colors.dark.text.primary,
      secondary: colors.dark.text.secondary,
    },
    divider: colors.dark.border.primary,
  },
  shadows: [
    'none',
    `0px 1px 2px ${colors.shadows.dark.xs}`,
    `0px 1px 3px ${colors.shadows.dark.sm}`,
    `0px 1px 5px ${colors.shadows.dark.sm}`,
    `0px 1px 8px ${colors.shadows.dark.md}`,
    `0px 1px 10px ${colors.shadows.dark.md}`,
    `0px 1px 12px ${colors.shadows.dark.md}`,
    `0px 2px 4px ${colors.shadows.dark.sm}`,
    `0px 2px 6px ${colors.shadows.dark.md}`,
    `0px 2px 8px ${colors.shadows.dark.md}`,
    `0px 2px 10px ${colors.shadows.dark.lg}`,
    `0px 2px 12px ${colors.shadows.dark.lg}`,
    `0px 3px 6px ${colors.shadows.dark.md}`,
    `0px 3px 8px ${colors.shadows.dark.lg}`,
    `0px 3px 10px ${colors.shadows.dark.lg}`,
    `0px 3px 12px ${colors.shadows.dark.xl}`,
    `0px 4px 8px ${colors.shadows.dark.lg}`,
    `0px 4px 10px ${colors.shadows.dark.lg}`,
    `0px 4px 12px ${colors.shadows.dark.xl}`,
    `0px 4px 16px ${colors.shadows.dark.xl}`,
    `0px 5px 10px ${colors.shadows.dark.lg}`,
    `0px 5px 12px ${colors.shadows.dark.xl}`,
    `0px 5px 16px ${colors.shadows.dark.xl}`,
    `0px 6px 12px ${colors.shadows.dark.xl}`,
    `0px 6px 16px ${colors.shadows.dark.xl}`,
  ],
  components: {
    ...themeOptions.components,
    MuiTypography: {
      styleOverrides: {
        root: {
          color: colors.dark.text.primary,
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
          borderRadius: 6,
          backgroundColor: colors.dark.background.paper,
          '&:hover': {
            boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
          },
        },
      },
    },
    MuiButton: {
      defaultProps: {
        disableRipple: true,
      },
      styleOverrides: {
        root: {
          borderRadius: 6,
          textTransform: 'none',
          fontWeight: 500,
          minWidth: 'auto',
          // padding: '8px 16px',
          paddingLeft: 16,
          paddingRight: 16,
          '& .MuiSvgIcon-root': {
            fontSize: '1.25rem',
          },
        },
        contained: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;

          if (colorKeySafe === 'primary') {
            const isDark = theme.palette.mode === 'dark';
            return {
              boxShadow: 'none',
              color: theme.palette.primary.contrastText,
              background: `linear-gradient(45deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
              transition: 'opacity 0.3s ease-in-out',
              '&:hover': {
                boxShadow: 'none',
                opacity: 0.9,
              },
              '&:disabled': {
                opacity: 0.6,
                color: alpha(theme.palette.primary.contrastText, 0.6),
                background: isDark
                  ? `linear-gradient(45deg, ${theme.palette.primary.dark}, ${theme.palette.secondary.dark})`
                  : `linear-gradient(45deg, ${theme.palette.primary.light}, ${theme.palette.secondary.light})`,
              },
            };
          }

          return {
            boxShadow: 'none',
            color: palette.contrastText,
            backgroundColor: palette.main,
            transition: 'opacity 0.3s ease-in-out',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: palette.dark || palette.main,
            },
            '&:disabled': {
              opacity: 0.6,
              color: alpha(palette.contrastText, 0.6),
              backgroundColor: alpha(palette.main, 0.4),
            },
          };
        },
        outlined: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;
          return {
            boxShadow: 'none',
            color: palette.main,
            border: `1px solid ${alpha(palette.main, 0.5)}`,
            backgroundColor: 'transparent',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: alpha(palette.main, 0.10),
              border: `1px solid ${palette.main}`,
            },
          };
        },
        text: ({ ownerState, theme }) => {
          const colorKey = ownerState.color || 'primary';
          const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
          const palette =
            colorKeySafe === 'secondary' ? theme.palette.secondary :
            colorKeySafe === 'success' ? theme.palette.success :
            colorKeySafe === 'error' ? theme.palette.error :
            colorKeySafe === 'warning' ? theme.palette.warning :
            colorKeySafe === 'info' ? theme.palette.info :
            theme.palette.primary;
          
          return {
            boxShadow: 'none',
            color: palette.main,
            backgroundColor: 'transparent',
            '&:hover': {
              boxShadow: 'none',
              backgroundColor: alpha(palette.main, 0.10),
            },
          };
        },
      },
    },
    MuiButtonGroup: {
      styleOverrides: {
        root: {
          border: `1px solid ${colors.dark.border.primary}`,
          borderRadius: 8,
          '& .MuiButton-root': {
            '&:not(:last-child)': {
              borderRight: `none`,
            },
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          fontWeight: 500,
        },
      },
    },
    MuiIconButton: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiMenuItem: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiButtonBase: {
      defaultProps: {
        disableRipple: true,
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          marginTop: 24,
          marginBottom: 24,
          backgroundColor: colors.dark.background.default,
          padding: 4,
          paddingRight: 8,
          paddingLeft: 8,
          borderRadius: 6,
          '& .MuiInputLabel-root': {
            position: 'absolute',
            transform: 'translateY(-150%)',
            fontWeight: 500,
            fontSize: 14,
            color: colors.dark.text.secondary,
          },
          '& .MuiFormHelperText-root': {
            position: 'absolute',
            bottom: 0,
            left: -12,
            transform: 'translateY(150%)',
            fontWeight: 400,
            color: colors.dark.text.secondary,
            marginTop: 4,
          },
          transition: 'all 0.2s ease-in-out',
          border: `1px solid ${colors.dark.border.primary}`,
          '&:hover': {
            backgroundColor: colors.dark.background.paper,
          },
          '&:focus-within': {
            backgroundColor: colors.dark.background.paper,
            border: `1px solid ${colors.primary.main}`,
          },  
          '& .MuiInput-underline:before': {
            borderBottom: 'none',
          },
          '& .MuiInput-underline:after': {
            borderBottom: 'none',
          },
          '& .MuiInput-underline:hover:not(.Mui-disabled):before': {
            borderBottom: 'none',
          },
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              border: 'none',
            },
            '&:hover fieldset': {
              border: 'none',
            },
            '&.Mui-focused fieldset': {
              border: 'none',
            },
          },
          '& .MuiInputBase-input': {
            color: colors.dark.text.primary,
            padding: 3,
          },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
          backgroundColor: colors.dark.background.paper,
        },
        elevation1: {
          boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
        },
        elevation2: {
          boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
        },
        elevation3: {
          boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: colors.dark.background.paper,
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          boxShadow: 'none',
          backgroundColor: colors.dark.background.paper,
          borderRight: `1px solid ${colors.dark.border.primary}`,
        },
      },
    },
    MuiListItemButton: {
      defaultProps: {
        disableRipple: true,
      },
      styleOverrides: {
        root: {
          borderRadius: 6,
          margin: '4px 8px',
          '&.Mui-selected': {
            backgroundColor: colors.primary.main,
            // color: colors.primary.contrastText,
            '&:hover': {
              backgroundColor: colors.primary.dark,
            },
          },
          '&:hover': {
            backgroundColor: colors.dark.background.default,
          },
        },
      },
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          backgroundColor: colors.dark.background.default,
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          borderBottom: `1px solid ${colors.dark.border.primary}`,
          '&:hover': {
            backgroundColor: colors.dark.background.default,
          },
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          boxShadow: 'none',
          border: `1px solid ${colors.dark.border.primary}`,
          borderRadius: 6,
          padding: 12,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 8px 16px ${colors.shadows.dark.lg}`,
          borderRadius: 6,
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 4px 8px ${colors.shadows.dark.lg}`,
          borderRadius: 6,
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          boxShadow: `0px 4px 8px ${colors.shadows.dark.lg}`,
          borderRadius: 6,
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          boxShadow: `0px 2px 6px ${colors.shadows.dark.lg}`,
          borderRadius: 6,
        },
      },
    },
  },
});

// Export theme options and colors for customization
export { themeOptions, colors };

// Default export
export default aiAgentTheme;
