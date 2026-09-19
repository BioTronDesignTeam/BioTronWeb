import { useEffect } from 'react';
import type { Scope } from '../types';
import { CalendarToolbar } from './PublicCalendar';
import { ScopeFilter } from './ScopeFilter';

interface SidebarProps {
  open: boolean;
  month: Date;
  onMonthChange: (month: Date) => void;
  /** Shown to editors only. */
  onManage?: () => void;
  scopes: Scope[];
  selectedScopes: string[];
  onScopeChange: (scopeIds: string[]) => void;
  onSubscribe: () => void;
  onClose: () => void;
}

/**
 * The month, the calendar filter, Subscribe, and Manage. From lg up it sits beside the grid and
 * pushes it over. Below lg there is no room, so it slides over the page as a
 * drawer with a backdrop, and Escape or a tap outside closes it.
 */
export function Sidebar({ open, month, onMonthChange, onManage, scopes, selectedScopes, onScopeChange, onSubscribe, onClose }: SidebarProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      // On a wide screen the sidebar is part of the page, not a layer to dismiss.
      if (event.key === 'Escape' && !window.matchMedia('(min-width: 1024px)').matches) onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <>
      <button type="button" aria-label="Close sidebar" onClick={onClose} className="fixed inset-0 z-40 cursor-default bg-ink/40 lg:hidden dark:bg-black/60" />
      <aside
        id="calendar-sidebar"
        aria-label="Calendars"
        className="fixed inset-y-0 left-0 z-50 flex w-72 max-w-[85vw] flex-col gap-4 overflow-y-auto border-r border-ink/10 bg-white p-4 pt-[calc(1rem+env(safe-area-inset-top,0px))] shadow-xl dark:border-line dark:bg-surface lg:static lg:z-auto lg:w-64 lg:max-w-none lg:shrink-0 lg:pt-4 lg:shadow-none"
      >
        {/* As a drawer it hides the grid, and the header keeps the month in view, so the month shows here only when docked. */}
        <div className="hidden lg:block">
          <CalendarToolbar month={month} onMonthChange={onMonthChange} inSidebar />
        </div>
        <ScopeFilter scopes={scopes} selected={selectedScopes} onChange={onScopeChange} />
        <button type="button" onClick={onSubscribe} className="min-h-11 shrink-0 whitespace-nowrap rounded-full bg-deep px-5 text-sm font-semibold text-white hover:bg-brand dark:bg-brand dark:hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand dark:focus-visible:outline-link">
          Subscribe to a calendar
        </button>
        {onManage && (
          <button type="button" onClick={onManage} className="min-h-11 shrink-0 whitespace-nowrap rounded-full border border-ink/15 px-5 text-sm font-semibold text-brand hover:bg-soft/30 dark:border-line-strong dark:text-link dark:hover:bg-white/10">
            Manage
          </button>
        )}
      </aside>
    </>
  );
}
