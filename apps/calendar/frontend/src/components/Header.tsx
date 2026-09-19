import { useState, type ReactNode } from 'react';
import { Brand, Button, ThemeToggle, UserMenu } from '@biotron/style';
import { login, logout } from '../api';
import type { AuthStatus } from '../types';

interface HeaderProps {
  auth: AuthStatus;
  managing: boolean;
  onPublic: () => void;
  onLoggedOut: () => void;
  /**
   * The month controls, as a second row below lg. There the sidebar is a
   * drawer that covers the calendar, so it cannot hold them. From lg up they
   * live in the sidebar only, and closing it puts them away with it.
   */
  toolbar?: ReactNode;
  /** Day, Week, or Month. It sits beside the theme switch from sm up; a phone has no room, so the sidebar shows it there. */
  viewSwitch?: ReactNode;
  /** Present on the calendar view, where the button opens and closes the sidebar. */
  sidebar?: { open: boolean; filterCount: number; onToggle: () => void };
}

export function Header({ auth, managing, onPublic, onLoggedOut, toolbar, viewSwitch, sidebar }: HeaderProps) {
  const operator = auth.operator;
  const [logoutError, setLogoutError] = useState('');
  return (
    <header className="sticky top-0 z-40 border-b border-ink/10 bg-white/90 pt-[env(safe-area-inset-top,0px)] backdrop-blur-xl dark:border-line-strong dark:bg-surface/90">
      {/* The calendar view runs edge to edge, so its bar does too. Other views keep the centred column. */}
      <div className={`mx-auto flex min-h-16 flex-wrap items-center gap-x-3 px-4 lg:flex-nowrap ${sidebar ? 'lg:px-5' : 'max-w-[1500px] sm:px-6 lg:px-8'}`}>
        <div className="flex min-h-16 min-w-0 items-center gap-2 sm:gap-3">
          {sidebar && (
            <button
              type="button"
              onClick={sidebar.onToggle}
              aria-expanded={sidebar.open}
              aria-controls="calendar-sidebar"
              aria-label={sidebar.filterCount === 0 ? 'Toggle sidebar' : `Toggle sidebar, ${sidebar.filterCount} calendars selected`}
              className="relative -ml-2 grid size-11 shrink-0 place-items-center rounded-full hover:bg-soft/25 dark:hover:bg-white/10"
            >
              <svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                <line x1="3" y1="6" x2="19" y2="6" />
                <line x1="3" y1="11" x2="19" y2="11" />
                <line x1="3" y1="16" x2="19" y2="16" />
              </svg>
              {/* The filter lives in the sidebar, so a closed sidebar still has to say a filter is on. */}
              {sidebar.filterCount > 0 && (
                <span className="absolute right-0 top-0 grid h-5 min-w-5 place-items-center rounded-full bg-brand px-1 text-[11px] font-bold leading-none text-white">{sidebar.filterCount}</span>
              )}
            </button>
          )}
          <Brand className="w-24 sm:w-28" />
          <span className="hidden h-6 w-px bg-ink/15 sm:block dark:bg-white/20" aria-hidden="true" />
          <span className="hidden truncate text-sm font-semibold tracking-wide sm:block">Calendar</span>
        </div>
        {toolbar && <div className="order-last w-full pb-3 lg:hidden">{toolbar}</div>}
        <div className="ml-auto flex items-center gap-2">
          {/* On the calendar view Manage lives in the sidebar. This button is the way back from the Manage view. */}
          {auth.can_write && managing && (
            <button
              type="button"
              className="min-h-11 rounded-full px-4 text-sm font-semibold text-brand hover:bg-soft/30 dark:text-link dark:hover:bg-white/10"
              onClick={onPublic}
            >
              <span className="sm:hidden">View</span><span className="hidden sm:inline">View calendar</span>
            </button>
          )}
          {viewSwitch && <div className="hidden sm:block">{viewSwitch}</div>}
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
