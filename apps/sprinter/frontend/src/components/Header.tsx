import { useState } from 'react';
import { Brand, Button, ThemeToggle, UserMenu } from '@biotron/style';
import { login, logout } from '../api';
import type { AuthStatus } from '../types';

interface HeaderProps {
  auth: AuthStatus;
  onLoggedOut: () => void;
}

export function Header({ auth, onLoggedOut }: HeaderProps) {
  const operator = auth.operator;
  const [logoutError, setLogoutError] = useState('');
  return (
    <header className="sticky top-0 z-40 border-b border-ink/10 bg-white/90 pt-[env(safe-area-inset-top,0px)] backdrop-blur-xl dark:border-line-strong dark:bg-surface/90">
      <div className="mx-auto flex min-h-16 max-w-[1200px] items-center gap-3 px-4 sm:px-6 lg:px-8">
        <div className="flex min-h-11 min-w-0 items-center gap-3">
          <Brand className="w-28" />
          <span className="hidden h-6 w-px bg-ink/15 sm:block dark:bg-white/20" aria-hidden="true" />
          <span className="hidden truncate text-sm font-semibold tracking-wide sm:block">Sprinter</span>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          {logoutError && <span className="fixed right-4 top-20 z-50 max-w-xs rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 shadow-lg dark:bg-red-950 dark:text-red-200" role="alert">{logoutError}</span>}
          {operator ? (
            <UserMenu
              user={{ name: operator.name, login: operator.login, avatarUrl: operator.avatar_url }}
              onLogout={async () => {
                setLogoutError('');
                try {
                  await logout();
                  onLoggedOut();
                } catch (error) {
                  setLogoutError(error instanceof Error ? error.message : 'Sign out failed');
                }
              }}
            />
          ) : (
            <Button onClick={login} className="min-h-11 whitespace-nowrap">Log in</Button>
          )}
        </div>
      </div>
    </header>
  );
}
