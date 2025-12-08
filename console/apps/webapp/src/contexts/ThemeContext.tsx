import React, { createContext, useContext, useState, useEffect, useMemo } from 'react';

type ThemeMode = 'light' | 'dark' | 'system';
type ActualTheme = 'light' | 'dark';

interface ThemeContextType {
  mode: ThemeMode;
  actualTheme: ActualTheme;
  setMode: (mode: ThemeMode) => void;
  toggleTheme: () => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const THEME_STORAGE_KEY = 'theme-mode';

/**
 * Get the system color scheme preference
 */
const getSystemTheme = (): ActualTheme => {
  if (typeof window !== 'undefined') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return 'light';
};

/**
 * Get the stored theme preference from localStorage
 */
const getStoredTheme = (): ThemeMode => {
  if (typeof window !== 'undefined') {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === 'light' || stored === 'dark' || stored === 'system') {
      return stored;
    }
  }
  return 'system';
};

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // Initialize mode from localStorage or default to 'system'
  const [mode, setMode] = useState<ThemeMode>(getStoredTheme);
  
  // Track system theme preference
  const [systemTheme, setSystemTheme] = useState<ActualTheme>(getSystemTheme);

  // Listen to system theme changes
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    
    const handleChange = (event: MediaQueryListEvent) => {
      setSystemTheme(event.matches ? 'dark' : 'light');
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  // Calculate the actual theme based on mode and system preference
  const actualTheme: ActualTheme = useMemo(() => {
    if (mode === 'system') {
      return systemTheme;
    }
    return mode;
  }, [mode, systemTheme]);

  // Apply theme to document body and store preference
  useEffect(() => {
    const body = document.body;
    
    if (actualTheme === 'dark') {
      body.classList.add('dark-mode');
    } else {
      body.classList.remove('dark-mode');
    }

    // Store user preference
    localStorage.setItem(THEME_STORAGE_KEY, mode);
  }, [actualTheme, mode]);

  const handleSetMode = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  const toggleTheme = () => {
    // Toggle between light and dark (system mode requires explicit selection)
    setMode(prevMode => {
      if (prevMode === 'system') {
        // If in system mode, toggle to the opposite of current actual theme
        return actualTheme === 'dark' ? 'light' : 'dark';
      }
      return prevMode === 'light' ? 'dark' : 'light';
    });
  };

  const value = useMemo(
    () => ({
      mode,
      actualTheme,
      setMode: handleSetMode,
      toggleTheme,
    }),
    [mode, actualTheme]
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
};

/**
 * Hook to use the theme context
 */
export const useTheme = (): ThemeContextType => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
};

