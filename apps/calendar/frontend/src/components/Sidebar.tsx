import { useEffect, useRef, useSyncExternalStore, type ReactNode } from 'react';
import type { CalendarView } from '../date';
import type { Scope } from '../types';
import { CalendarToolbar } from './PublicCalendar';
import { ScopeFilter } from './ScopeFilter';

interface SidebarProps {
  open: boolean;
  view: CalendarView;
  anchor: Date;
  onStep: (direction: -1 | 1) => void;
  /** The header has no room for it on a phone, so it shows here below sm. */
  viewSwitch: ReactNode;
  /** Shown to editors only. */
  onManage?: () => void;
  scopes: Scope[];
  selectedScopes: string[];
  onScopeChange: (scopeIds: string[]) => void;
  onSubscribe: () => void;
  onClose: () => void;
}

const wide = '(min-width: 1024px)';

/** True below lg, where the sidebar is a drawer over the page and not a column beside the grid. */
function useIsDrawer() {
  return useSyncExternalStore(
    (notify) => {
      const query = window.matchMedia(wide);
      query.addEventListener('change', notify);
      return () => query.removeEventListener('change', notify);
    },
    () => !window.matchMedia(wide).matches,
  );
}

const focusableSelector = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * The month, the calendar filter, Subscribe, and Manage. From lg up it sits beside the grid and
 * pushes it over. Below lg there is no room, so it slides over the page as a
 * drawer with a backdrop, and Escape or a tap outside closes it.
 */
export function Sidebar({ open, view, anchor, onStep, viewSwitch, onManage, scopes, selectedScopes, onScopeChange, onSubscribe, onClose }: SidebarProps) {
  const drawer = useIsDrawer();
  const dialog = useRef<HTMLDivElement>(null);
  const panel = useRef<HTMLElement>(null);
  // The parent passes a new onClose on every render. Reading it through a ref
  // keeps it out of the effect below, which would otherwise rerun on each
  // render and throw focus back to the panel while someone ticks a checkbox.
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  });

  // As a drawer the sidebar is a modal layer, so it behaves like one: focus
  // moves into it, Tab stays inside it, Escape closes it, and focus goes back
  // to the menu button afterwards. Beside the grid it is part of the page and
  // does none of this.
  useEffect(() => {
    if (!open || !drawer) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onCloseRef.current();
        return;
      }
      if (event.key !== 'Tab' || !dialog.current) return;
      const focusable = [...dialog.current.querySelectorAll<HTMLElement>(focusableSelector)]
        // Not offsetParent: it is null for a fixed element, which would drop the backdrop's Close button.
        .filter((element) => element.getClientRects().length > 0);
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const inside = dialog.current.contains(document.activeElement);
      if (event.shiftKey && (!inside || document.activeElement === first || document.activeElement === panel.current)) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (!inside || document.activeElement === last)) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    panel.current?.focus();
    return () => {
      document.removeEventListener('keydown', onKey);
      previousFocus?.focus();
    };
  }, [open, drawer]);

  if (!open) return null;
  return (
    // The dialog wraps the backdrop too, so a screen reader that stays inside
    // the dialog can still reach "Close sidebar". From lg up the wrapper has no
    // box of its own and the sidebar sits in the page's row as before.
    <div ref={dialog} role={drawer ? 'dialog' : undefined} aria-modal={drawer ? true : undefined} aria-label={drawer ? 'Calendars' : undefined} className="lg:contents">
      <aside
        ref={panel}
        id="calendar-sidebar"
        aria-label="Calendars"
        tabIndex={-1}
        className="fixed inset-y-0 left-0 z-50 flex w-72 outline-none max-w-[85vw] flex-col gap-4 overflow-hidden border-r border-ink/10 bg-white p-4 pt-[calc(1rem+env(safe-area-inset-top,0px))] shadow-xl dark:border-line dark:bg-surface lg:static lg:z-auto lg:w-64 lg:max-w-none lg:shrink-0 lg:pt-4 lg:shadow-none pb-[calc(1rem+env(safe-area-inset-bottom,0px))]"
      >
        {/* As a drawer it hides the grid, and the header keeps the month in view, so the month shows here only when docked. */}
        <div className="hidden lg:block">
          <CalendarToolbar view={view} anchor={anchor} onStep={onStep} inSidebar />
        </div>
        <div className="sm:hidden">{viewSwitch}</div>
        {/* Only the calendar list scrolls, so a long list never pushes the buttons off the bottom. */}
        <div className="-mx-2 min-h-0 flex-1 overflow-y-auto px-2">
          <ScopeFilter scopes={scopes} selected={selectedScopes} onChange={onScopeChange} />
        </div>
        <div className="flex shrink-0 flex-col gap-2">
          <button type="button" onClick={onSubscribe} className="min-h-11 shrink-0 whitespace-nowrap rounded-full bg-deep px-5 text-sm font-semibold text-white hover:bg-brand dark:bg-brand dark:hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand dark:focus-visible:outline-link">
            Subscribe to a calendar
          </button>
          {onManage && (
            <button type="button" onClick={onManage} className="min-h-11 shrink-0 whitespace-nowrap rounded-full border border-ink/15 px-5 text-sm font-semibold text-brand hover:bg-soft/30 dark:border-line-strong dark:text-link dark:hover:bg-white/10">
              Manage
            </button>
          )}
        </div>
      </aside>
      {/* After the panel in the markup, so Tab reaches the controls first. It is drawn under the panel all the same. */}
      <button type="button" aria-label="Close sidebar" onClick={onClose} className="fixed inset-0 z-40 cursor-default bg-ink/40 lg:hidden dark:bg-black/60" />
    </div>
  );
}
