import type { OrgMember } from '../api';
import { panelHeading } from './ui';

function memberSuffix(member: OrgMember): string {
  return [
    member.is_banned ? ' · banned' : '',
    member.is_manager ? ' · manager' : '',
    member.is_superuser ? ' · superuser' : '',
  ].join('');
}

export default function OrgMemberList({
  members,
  selectedMemberId,
  onSelect,
}: {
  members: OrgMember[];
  selectedMemberId: number | null;
  onSelect: (memberId: number) => void;
}) {
  return (
    <div className="border-b border-slate-200 lg:border-b-0 lg:border-r dark:border-white/10">
      <div className="border-b border-slate-200 px-4 py-4 dark:border-white/10">
        <h2 className={panelHeading}>Members</h2>
      </div>
      {members.length === 0 ? (
        <p className="px-4 py-6 text-sm text-slate-500 dark:text-muted">No members found.</p>
      ) : (
        // A nested scroll region is miserable on touch, so on a phone the list
        // grows with the page and only becomes its own scroller on desktop.
        <ul className="lg:max-h-[32rem] lg:overflow-y-auto">
          {members.map((member) => (
            <li key={member.github_id}>
              <button
                type="button"
                onClick={() => onSelect(member.github_id)}
                className={`flex w-full min-h-11 items-center gap-3 px-4 py-3 text-left transition ${
                  selectedMemberId === member.github_id
                    ? 'bg-slate-100 dark:bg-surface-2'
                    : 'hover:bg-slate-50 dark:hover:bg-surface-2/50'
                }`}
              >
                {member.avatar_url ? (
                  <img
                    src={member.avatar_url}
                    alt=""
                    className="h-8 w-8 shrink-0 rounded-full object-cover ring-1 ring-slate-200 dark:ring-white/10"
                  />
                ) : (
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-200 text-xs font-medium text-slate-600 dark:bg-surface-2 dark:text-muted">
                    {member.login.slice(0, 1).toUpperCase()}
                  </div>
                )}
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-slate-900 dark:text-white">
                    {member.name || member.login}
                  </div>
                  <div className="truncate text-xs text-slate-500 dark:text-muted">
                    @{member.login}
                    {memberSuffix(member)}
                  </div>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
