import type { ReactNode } from 'react';

export type Tab = 'access' | 'keys' | 'org';

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex min-h-11 items-center justify-center rounded-lg px-3 py-2.5 text-sm font-medium transition sm:min-h-0 sm:px-4 ${
        active
          ? 'bg-white text-slate-900 shadow dark:bg-highlight dark:text-white'
          : 'text-slate-600 hover:text-slate-800 dark:text-muted dark:hover:text-white'
      }`}
    >
      {children}
    </button>
  );
}

/**
 * Staff see three tabs, which fit in one row at every width the header
 * supports. A member has only their own access to look at, so for them there
 * is no bar at all: a single tab with nothing to switch to is noise, and the
 * panel underneath already carries its own heading.
 */
export default function TabBar({
  tab,
  onSelect,
  isStaff,
}: {
  tab: Tab;
  onSelect: (tab: Tab) => void;
  isStaff: boolean;
}) {
  if (!isStaff) return null;
  return (
    <div className="mb-6 grid grid-cols-3 gap-1 rounded-xl bg-slate-200/80 p-1 ring-1 ring-slate-300 dark:bg-surface-2/80 dark:ring-white/10">
      <TabButton active={tab === 'access'} onClick={() => onSelect('access')}>
        My access
      </TabButton>
      <TabButton active={tab === 'keys'} onClick={() => onSelect('keys')}>
        Keys
      </TabButton>
      <TabButton active={tab === 'org'} onClick={() => onSelect('org')}>
        Org
      </TabButton>
    </div>
  );
}
