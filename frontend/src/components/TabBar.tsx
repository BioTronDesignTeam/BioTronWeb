import type { ReactNode } from 'react';

export type Tab = 'access' | 'approvals' | 'keys' | 'org';

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
      className={`inline-flex items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium transition ${
        active
          ? 'bg-white text-slate-900 shadow dark:bg-slate-100'
          : 'text-slate-600 hover:text-slate-800 dark:text-slate-300 dark:hover:text-slate-100'
      }`}
    >
      {children}
    </button>
  );
}

export default function TabBar({
  tab,
  onSelect,
  isStaff,
  pendingCount,
}: {
  tab: Tab;
  onSelect: (tab: Tab) => void;
  isStaff: boolean;
  pendingCount: number;
}) {
  return (
    <div
      className={`mb-6 rounded-xl bg-slate-200/80 p-1 ring-1 ring-slate-300 dark:bg-slate-800/80 dark:ring-white/10 ${
        isStaff ? 'grid gap-1' : 'inline-flex'
      }`}
      style={isStaff ? { gridTemplateColumns: 'repeat(4, minmax(0, 1fr))' } : undefined}
    >
      <TabButton active={tab === 'access'} onClick={() => onSelect('access')}>
        My access
      </TabButton>
      {isStaff && (
        <>
          <TabButton active={tab === 'approvals'} onClick={() => onSelect('approvals')}>
            Approvals{pendingCount ? ` (${pendingCount})` : ''}
          </TabButton>
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
