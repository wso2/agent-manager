import { useState, useEffect } from 'react';

/**
 * Custom hook to detect system color scheme preference
 * Returns true if dark mode is preferred, false for light mode
 */
export function useSystemTheme(): {
  isDark: boolean, 
  setIsDark: React.Dispatch<React.SetStateAction<boolean>>
} {
  const [isDark, setIsDark] = useState<boolean>(() => {
    // Check if window is available (client-side)
    if (typeof window !== 'undefined') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    // Default to light mode for SSR
    return false;
  });

  useEffect(() => {
    // Check if window is available (client-side)
    if (typeof window === 'undefined') return;

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    
    // Update state when preference changes
    const handleChange = (event: MediaQueryListEvent) => {
      setIsDark(event.matches);
    };

    // Listen for changes
    mediaQuery.addEventListener('change', handleChange);

    // Cleanup
    return () => {
      mediaQuery.removeEventListener('change', handleChange);
    };
  }, []);

  return {isDark, setIsDark};
}
