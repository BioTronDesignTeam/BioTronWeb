import { useState } from 'react';
import { Brand, Button, ThemeToggle, UserMenu } from '@biotron/style';
import { SITE_URL, login, logout } from '../api';
import type { AuthStatus } from '../types';

interface HeaderProps {
  auth: AuthStatus;
  managing: boolean;
  onManage: () => void;
  onPublic: () => void;
  onLoggedOut: () => void;
}

export function Header({ auth, managing, onManage, onPublic, onLoggedOut }: HeaderProps) {
  const operator = auth.operator;
  const [logoutError, setLogoutError] = useState('');
  return (
    <header className="sticky top-0 z-40 border-b border-ink/10 bg-white/90 backdrop-blur-xl dark:border-white/10 dark:bg-ink/90">
      <div className="mx-auto flex min-h-16 max-w-[1500px] items-center gap-3 px-4 sm:px-6 lg:px-8">
        <a href={SITE_URL} className="flex min-w-0 items-center gap-3 rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-brand" aria-label="Back to BioTron">
          <Brand className="hidden w-28 sm:inline-flex" />
          <Brand compact className="sm:hidden" />
          <span className="hidden h-6 w-px bg-ink/15 sm:block dark:bg-white/20" aria-hidden="true" />
          <span className="hidden truncate text-sm font-semibold tracking-wide sm:block">Calendar</span>
        </a>
        <div className="ml-auto flex items-center gap-2">
          {auth.can_write && (
            <button
              type="button"
              className="min-h-11 rounded-full px-4 text-sm font-semibold text-brand hover:bg-soft/30 dark:text-soft dark:hover:bg-white/10"
              onClick={managing ? onPublic : onManage}
            >
              {managing ? <><span className="sm:hidden">View</span><span className="hidden sm:inline">View calendar</span></> : 'Manage'}
            </button>
          )}
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
