import { createTheme, ThemeOptions, alpha } from '@wso2/oxygen-ui';
// import { theme } from 'src';

// Color palette for AI Agent Management Platform
const colors = {
  // Legacy color mappings for backward compatibility
  primary: {
    main: '#6e6af0',
    light: '#e1e1fe',
    dark: '#1e1955',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#6d6ae8',
    light: '#9896f0',
    dark: '#4744b8',
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
  },
  typography: {
    fontFamily: "Poppins, sans-serif",
    h1: {
      fontSize: '2.5rem',
      fontWeight: 600,
      lineHeight: 1.2,
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 600,
      lineHeight: 1.3,
    },
    h3: {
      fontSize: '1.75rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h4: {
      fontSize: '1.5rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h5: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.5,
    },
    h6: {
      fontSize: '1rem',
      fontWeight: 600,
      lineHeight: 1.6,
    },
    body1: {
      fontSize: '1rem',
      lineHeight: 1.5,
    },
    body2: {
      fontSize: '0.875rem',
      lineHeight: 1.43,
    },
    button: {
      fontSize: '0.875rem',
      fontWeight: 500,
      textTransform: 'none',
    },
    caption: {
      fontSize: '0.75rem',
      lineHeight: 1.4,
    },
  },
  shape: {
    borderRadius: 3,
  },
  spacing: 8, // 8px base spacing unit
  components: {
    // MuiCard: {
    //   styleOverrides: {
    //     root: {
    //       boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
    //       borderRadius: 10,
    //       backgroundColor: colors.light.background.paper,
    //       '&:hover': {
    //         boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
    //       },
    //     },
    //   },
    // },
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
          borderRadius: 4,
          fontWeight: 100,
        },
        "colorDefault": {
          // backgroundColor: alpha(colors.light.background.elevated, 0.1),
          border: `1px solid ${alpha(colors.primary.light, 0.1)}`,
          "&:hover": {
            border: `1px solid ${alpha(colors.primary.light, 0.1)}`,
          },
        },
      },
    },
    MuiIconButton: {
      defaultProps: {
        sx: {
          transition: "all 0.2s ease-in-out",
          borderRadius: "15%",
          border: `1px solid ${alpha(colors.primary.light, 0)}`,
          "&:hover": {
            border: `1px solid ${alpha(colors.primary.light, 0.1)}`,
            // backgroundColor: alpha(colors.light.background.elevated, 0.1),
          },
        },
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
          // border: `1px solid ${colors.light.border.primary}`,
          borderRadius: 6,
          padding: 12,
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          // boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
          // backgroundColor: colors.light.background.paper,
        },
        elevation1: {
          // boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
        elevation2: {
          // boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
        elevation3: {
          // boxShadow: `0px 2px 4px ${colors.shadows.light.sm}`,
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: colors.primary.contrastText,
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          boxShadow: 'none',
          // backgroundColor: colors.light.background.paper,
          //  borderRight: `1px solid ${colors.light.border.primary}`,
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
            // backgroundColor: colors.light.background.default,
          },
        },
      },
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          // backgroundColor: colors.light.background.default,
        },
      },
    },
    MuiAvatar: {
      styleOverrides: {
        root: {
          // backgroundColor: colors.light.background.default,
        },
        rounded: {
          borderRadius: 8,
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          // borderBottom: `1px solid ${colors.light.border.primary}`,
          '&:hover': {
            // backgroundColor: colors.light.background.default,
          },
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          // boxShadow: `0px 8px 16px ${colors.shadows.light.lg}`,
          borderRadius: 6,
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          // boxShadow: `0px 4px 8px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          // boxShadow: `0px 4px 8px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          // boxShadow: `0px 2px 6px ${colors.shadows.light.md}`,
          borderRadius: 6,
        },
      },
    },
  },
};

// Create the theme
export const aiAgentTheme = createTheme(themeOptions);

// Create a dark theme variant
// export const aiAgentDarkTheme = createTheme({
//   ...themeOptions,
//   palette: {
//     ...themeOptions.palette,
//     mode: 'dark',
//     primary: colors.primary,
//     background: {
//       default: colors.dark.background.default,
//       paper: colors.dark.background.paper,
//     },
//     text: {
//       primary: colors.dark.text.primary,
//       secondary: colors.dark.text.secondary,
//     },
//     divider: colors.dark.border.primary,
//   },
//   shadows: [
//     'none',
//     `0px 1px 2px ${colors.shadows.dark.xs}`,
//     `0px 1px 3px ${colors.shadows.dark.sm}`,
//     `0px 1px 5px ${colors.shadows.dark.sm}`,
//     `0px 1px 8px ${colors.shadows.dark.md}`,
//     `0px 1px 10px ${colors.shadows.dark.md}`,
//     `0px 1px 12px ${colors.shadows.dark.md}`,
//     `0px 2px 4px ${colors.shadows.dark.sm}`,
//     `0px 2px 6px ${colors.shadows.dark.md}`,
//     `0px 2px 8px ${colors.shadows.dark.md}`,
//     `0px 2px 10px ${colors.shadows.dark.lg}`,
//     `0px 2px 12px ${colors.shadows.dark.lg}`,
//     `0px 3px 6px ${colors.shadows.dark.md}`,
//     `0px 3px 8px ${colors.shadows.dark.lg}`,
//     `0px 3px 10px ${colors.shadows.dark.lg}`,
//     `0px 3px 12px ${colors.shadows.dark.xl}`,
//     `0px 4px 8px ${colors.shadows.dark.lg}`,
//     `0px 4px 10px ${colors.shadows.dark.lg}`,
//     `0px 4px 12px ${colors.shadows.dark.xl}`,
//     `0px 4px 16px ${colors.shadows.dark.xl}`,
//     `0px 5px 10px ${colors.shadows.dark.lg}`,
//     `0px 5px 12px ${colors.shadows.dark.xl}`,
//     `0px 5px 16px ${colors.shadows.dark.xl}`,
//     `0px 6px 12px ${colors.shadows.dark.xl}`,
//     `0px 6px 16px ${colors.shadows.dark.xl}`,
//   ],
//   components: {
//     ...themeOptions.components,
//     MuiCard: {
//       styleOverrides: {
//         root: {
//           boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//           borderRadius: 6,
//           backgroundColor: colors.dark.background.paper,
//           '&:hover': {
//             boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//           },
//         },
//       },
//     },
//     MuiButton: {
//       defaultProps: {
//         disableRipple: true,
//       },
//       styleOverrides: {
//         root: {
//           borderRadius: 6,
//           textTransform: 'none',
//           fontWeight: 500,
//           minWidth: 'auto',
//           // padding: '8px 16px',
//           paddingLeft: 16,
//           paddingRight: 16,
//           '& .MuiSvgIcon-root': {
//             fontSize: '1.25rem',
//           },
//         },
//         contained: ({ ownerState, theme }) => {
//           const colorKey = ownerState.color || 'primary';
//           const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
//           const palette =
//             colorKeySafe === 'secondary' ? theme.palette.secondary :
//             colorKeySafe === 'success' ? theme.palette.success :
//             colorKeySafe === 'error' ? theme.palette.error :
//             colorKeySafe === 'warning' ? theme.palette.warning :
//             colorKeySafe === 'info' ? theme.palette.info :
//             theme.palette.primary;

//           if (colorKeySafe === 'primary') {
//             const isDark = theme.palette.mode === 'dark';
//             return {
//               boxShadow: 'none',
//               color: theme.palette.primary.contrastText,
//               background: `linear-gradient(45deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
//               transition: 'opacity 0.3s ease-in-out',
//               '&:hover': {
//                 boxShadow: 'none',
//                 opacity: 0.9,
//               },
//               '&:disabled': {
//                 opacity: 0.6,
//                 color: alpha(theme.palette.primary.contrastText, 0.6),
//                 background: isDark
//                   ? `linear-gradient(45deg, ${theme.palette.primary.dark}, ${theme.palette.secondary.dark})`
//                   : `linear-gradient(45deg, ${theme.palette.primary.light}, ${theme.palette.secondary.light})`,
//               },
//             };
//           }

//           return {
//             boxShadow: 'none',
//             color: palette.contrastText,
//             backgroundColor: palette.main,
//             transition: 'opacity 0.3s ease-in-out',
//             '&:hover': {
//               boxShadow: 'none',
//               backgroundColor: palette.dark || palette.main,
//             },
//             '&:disabled': {
//               opacity: 0.6,
//               color: alpha(palette.contrastText, 0.6),
//               backgroundColor: alpha(palette.main, 0.4),
//             },
//           };
//         },
//         outlined: ({ ownerState, theme }) => {
//           const colorKey = ownerState.color || 'primary';
//           const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
//           const palette =
//             colorKeySafe === 'secondary' ? theme.palette.secondary :
//             colorKeySafe === 'success' ? theme.palette.success :
//             colorKeySafe === 'error' ? theme.palette.error :
//             colorKeySafe === 'warning' ? theme.palette.warning :
//             colorKeySafe === 'info' ? theme.palette.info :
//             theme.palette.primary;
//           return {
//             boxShadow: 'none',
//             color: palette.main,
//             border: `1px solid ${alpha(palette.main, 0.5)}`,
//             backgroundColor: 'transparent',
//             '&:hover': {
//               boxShadow: 'none',
//               backgroundColor: alpha(palette.main, 0.10),
//               border: `1px solid ${palette.main}`,
//             },
//           };
//         },
//         text: ({ ownerState, theme }) => {
//           const colorKey = ownerState.color || 'primary';
//           const colorKeySafe = colorKey === 'inherit' ? 'primary' : colorKey;
//           const palette =
//             colorKeySafe === 'secondary' ? theme.palette.secondary :
//             colorKeySafe === 'success' ? theme.palette.success :
//             colorKeySafe === 'error' ? theme.palette.error :
//             colorKeySafe === 'warning' ? theme.palette.warning :
//             colorKeySafe === 'info' ? theme.palette.info :
//             theme.palette.primary;
          
//           return {
//             boxShadow: 'none',
//             color: palette.main,
//             backgroundColor: 'transparent',
//             '&:hover': {
//               boxShadow: 'none',
//               backgroundColor: alpha(palette.main, 0.10),
//             },
//           };
//         },
//       },
//     },
//     MuiButtonGroup: {
//       styleOverrides: {
//         root: {
//           border: `1px solid ${colors.dark.border.primary}`,
//           borderRadius: 8,
//           '& .MuiButton-root': {
//             '&:not(:last-child)': {
//               borderRight: `none`,
//             },
//           },
//         },
//       },
//     },
//     MuiChip: {
//       styleOverrides: {
//         root: {
//           borderRadius: 6,
//           fontWeight: 500,
//         },
//       },
//     },
//     MuiIconButton: {
//       defaultProps: {
//         disableRipple: true,
//       },
//     },
//     MuiMenuItem: {
//       defaultProps: {
//         disableRipple: true,
//       },
//     },
//     MuiButtonBase: {
//       defaultProps: {
//         disableRipple: true,
//       },
//     },
//     MuiTextField: {
//       styleOverrides: {
//         root: {
//           marginTop: 24,
//           marginBottom: 24,
//           backgroundColor: colors.dark.background.default,
//           padding: 4,
//           paddingRight: 8,
//           paddingLeft: 8,
//           borderRadius: 6,
//           '& .MuiInputLabel-root': {
//             position: 'absolute',
//             transform: 'translateY(-150%)',
//             fontWeight: 500,
//             fontSize: 14,
//             color: colors.dark.text.secondary,
//           },
//           '& .MuiFormHelperText-root': {
//             position: 'absolute',
//             bottom: 0,
//             left: -12,
//             transform: 'translateY(150%)',
//             fontWeight: 400,
//             color: colors.dark.text.secondary,
//             marginTop: 4,
//           },
//           transition: 'all 0.2s ease-in-out',
//           border: `1px solid ${colors.dark.border.primary}`,
//           '&:hover': {
//             backgroundColor: colors.dark.background.paper,
//           },
//           '&:focus-within': {
//             backgroundColor: colors.dark.background.paper,
//             border: `1px solid ${colors.primary.main}`,
//           },  
//           '& .MuiInput-underline:before': {
//             borderBottom: 'none',
//           },
//           '& .MuiInput-underline:after': {
//             borderBottom: 'none',
//           },
//           '& .MuiInput-underline:hover:not(.Mui-disabled):before': {
//             borderBottom: 'none',
//           },
//           '& .MuiOutlinedInput-root': {
//             '& fieldset': {
//               border: 'none',
//             },
//             '&:hover fieldset': {
//               border: 'none',
//             },
//             '&.Mui-focused fieldset': {
//               border: 'none',
//             },
//           },
//           '& .MuiInputBase-input': {
//             color: colors.dark.text.primary,
//             padding: 3,
//           },
//         },
//       },
//     },
//     MuiPaper: {
//       styleOverrides: {
//         root: {
//           boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//           backgroundColor: colors.dark.background.paper,
//         },
//         elevation1: {
//           boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//         },
//         elevation2: {
//           boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//         },
//         elevation3: {
//           boxShadow: `0px 2px 4px ${colors.shadows.dark.sm}`,
//         },
//       },
//     },
//     MuiAppBar: {
//       styleOverrides: {
//         root: {
//           backgroundColor: colors.dark.background.paper,
//         },
//       },
//     },
//     MuiDrawer: {
//       styleOverrides: {
//         paper: {
//           boxShadow: 'none',
//           backgroundColor: colors.dark.background.paper,
//           borderRight: `1px solid ${colors.dark.border.primary}`,
//         },
//       },
//     },
//     MuiListItemButton: {
//       defaultProps: {
//         disableRipple: true,
//       },
//       styleOverrides: {
//         root: {
//           borderRadius: 6,
//           margin: '4px 8px',
//           '&.Mui-selected': {
//             backgroundColor: colors.primary.main,
//             // color: colors.primary.contrastText,
//             '&:hover': {
//               backgroundColor: colors.primary.dark,
//             },
//           },
//           '&:hover': {
//             backgroundColor: colors.dark.background.default,
//           },
//         },
//       },
//     },
//     MuiTableHead: {
//       styleOverrides: {
//         root: {
//           backgroundColor: colors.dark.background.default,
//         },
//       },
//     },
//     MuiTableRow: {
//       styleOverrides: {
//         root: {
//           borderBottom: `1px solid ${colors.dark.border.primary}`,
//           '&:hover': {
//             backgroundColor: colors.dark.background.default,
//           },
//         },
//       },
//     },
//     MuiAlert: {
//       styleOverrides: {
//         root: {
//           boxShadow: 'none',
//           border: `1px solid ${colors.dark.border.primary}`,
//           borderRadius: 6,
//           padding: 12,
//         },
//       },
//     },
//     MuiDialog: {
//       styleOverrides: {
//         paper: {
//           boxShadow: `0px 8px 16px ${colors.shadows.dark.lg}`,
//           borderRadius: 6,
//         },
//       },
//     },
//     MuiMenu: {
//       styleOverrides: {
//         paper: {
//           boxShadow: `0px 4px 8px ${colors.shadows.dark.lg}`,
//           borderRadius: 6,
//         },
//       },
//     },
//     MuiPopover: {
//       styleOverrides: {
//         paper: {
//           boxShadow: `0px 4px 8px ${colors.shadows.dark.lg}`,
//           borderRadius: 6,
//         },
//       },
//     },
//     MuiTooltip: {
//       styleOverrides: {
//         tooltip: {
//           boxShadow: `0px 2px 6px ${colors.shadows.dark.lg}`,
//           borderRadius: 6,
//         },
//       },
//     },
//   },
// });

// Export theme options and colors for customization
export { themeOptions, colors };

// Default export
export default aiAgentTheme;
