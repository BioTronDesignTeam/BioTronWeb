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
          ? 'bg-white text-slate-900 shadow dark:bg-slate-100'
          : 'text-slate-600 hover:text-slate-800 dark:text-slate-300 dark:hover:text-slate-100'
      }`}
    >
      {children}
    </button>
  );
}

/**
 * Staff see three tabs, which fit in one row at every width the header
 * supports; a member sees only their own access.
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
  return (
    <div
      className={`mb-6 rounded-xl bg-slate-200/80 p-1 ring-1 ring-slate-300 dark:bg-slate-800/80 dark:ring-white/10 ${
        isStaff ? 'grid grid-cols-3 gap-1' : 'inline-flex'
      }`}
    >
      <TabButton active={tab === 'access'} onClick={() => onSelect('access')}>
        My access
      </TabButton>
      {isStaff && (
        <>
          <TabButton active={tab === 'keys'} onClick={() => onSelect('keys')}>
            Keys
          </TabButton>
          <TabButton active={tab === 'org'} onClick={() => onSelect('org')}>
            Org
          </TabButton>
        </>
      )}
    </div>
  );
}
