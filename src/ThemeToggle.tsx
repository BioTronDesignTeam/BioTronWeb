import { useEffect, useState } from 'react';

export type Theme = 'light' | 'dark';

function documentTheme(): Theme {
  if (typeof document === 'undefined') return 'light';
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

export function applyTheme(theme: Theme, persist = true) {
  if (typeof document === 'undefined') return;
  const dark = theme === 'dark';
  const root = document.documentElement;
  root.classList.toggle('dark', dark);
  root.style.colorScheme = theme;
  root.style.backgroundColor = dark ? '#16033c' : '#ffffff';
  if (persist) {
    try {
      localStorage.setItem('darkMode', String(dark));
    } catch {
      // Storage can be unavailable in private or hardened browser contexts.
    }
  }
}

const SunIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0-13a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0V5a1 1 0 0 1 1-1Zm0 14a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0v-1a1 1 0 0 1 1-1ZM5 11a1 1 0 1 1 0 2H4a1 1 0 1 1 0-2h1Zm15 0a1 1 0 1 1 0 2h-1a1 1 0 1 1 0-2h1ZM6.34 6.34a1 1 0 0 1 1.41 0l.71.71A1 1 0 0 1 7.05 8.46l-.71-.71a1 1 0 0 1 0-1.41Zm9.9 9.9a1 1 0 0 1 1.41 0l.71.71a1 1 0 0 1-1.41 1.41l-.71-.71a1 1 0 0 1 0-1.41ZM17.66 6.34a1 1 0 0 1 0 1.41l-.71.71a1 1 0 1 1-1.41-1.41l.71-.71a1 1 0 0 1 1.41 0ZM7.76 16.24a1 1 0 0 1 0 1.41l-.71.71a1 1 0 0 1-1.41-1.41l.71-.71a1 1 0 0 1 1.41 0Z" />
  </svg>
);

const MoonIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z" />
  </svg>
);

export type ThemeToggleProps = {
  className?: string;
};

export function ThemeToggle({ className = '' }: ThemeToggleProps) {
  const [theme, setTheme] = useState<Theme>(documentTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === 'darkMode') setTheme(event.newValue === 'true' ? 'dark' : 'light');
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  const dark = theme === 'dark';
  return (
    <button
      type="button"
      role="switch"
      aria-checked={dark}
      aria-label={`Color theme: ${theme} mode`}
      className={`biotron-theme-toggle ${dark ? 'biotron-theme-toggle--dark' : ''} ${className}`.trim()}
      onClick={() => setTheme(dark ? 'light' : 'dark')}
    >
      <span className="biotron-theme-toggle__thumb" aria-hidden="true" />
      <span className="biotron-theme-toggle__option"><SunIcon /></span>
      <span className="biotron-theme-toggle__option"><MoonIcon /></span>
    </button>
  );
}
