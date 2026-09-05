import { useEffect, useRef } from 'react';
import type { AppInfo, Grant, OrgMember, Permission } from '../api';
import type { OrgSelection } from '../hooks/useOrgSelection';
import OrgMemberDetail from './OrgMemberDetail';
import OrgMemberList from './OrgMemberList';
import { panel } from './ui';

export default function OrgTab({
  members,
  selectedMember,
  selection,
  memberGrantList,
  apps,
  permissions,
  canManage,
  onSelectMember,
  onSelectApp,
  onSelectPermission,
  onCreateGrant,
  onRevokeGrant,
  onBan,
  onUnban,
  onSetManager,
}: {
  members: OrgMember[];
  selectedMember: OrgMember | null;
  selection: OrgSelection;
  memberGrantList: Grant[];
  apps: AppInfo[];
  permissions: Permission[];
  canManage: boolean;
  onSelectMember: (memberId: number) => void;
  onSelectApp: (appId: string) => void;
  onSelectPermission: (permissionKey: string) => void;
  onCreateGrant: () => void;
  onRevokeGrant: (grant: Grant) => void;
  onBan: (id: number) => void;
  onUnban: (id: number) => void;
  onSetManager: (id: number, manager: boolean) => void;
}) {
  // Below the lg breakpoint the detail stacks under the member list, so a tap
  // on a member would otherwise change something off screen. Scroll it up
  // under the sticky header; on desktop the two sit side by side and nothing
  // moves.
  const detailRef = useRef<HTMLDivElement>(null);
  const selectedMemberId = selectedMember?.github_id ?? null;
  useEffect(() => {
    if (selectedMemberId === null) return;
    if (!window.matchMedia('(max-width: 1023px)').matches) return;
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    detailRef.current?.scrollIntoView({ block: 'start', behavior: reduceMotion ? 'auto' : 'smooth' });
  }, [selectedMemberId]);

  return (
    <section className={panel}>
      <div className="grid grid-cols-1 lg:min-h-[28rem] lg:grid-cols-[minmax(0,16rem)_1fr]">
        <OrgMemberList
          members={members}
          selectedMemberId={selection.memberId}
          onSelect={onSelectMember}
        />
        <div ref={detailRef} className="min-w-0 scroll-mt-20">
          {!selectedMember ? (
            <div className="flex h-full items-center justify-center px-6 py-16 text-sm text-slate-500 dark:text-muted">
              Select a member to manage access.
            </div>
          ) : (
            <OrgMemberDetail
              member={selectedMember}
              apps={apps}
              permissions={permissions}
              grants={memberGrantList}
              selection={selection}
              canManage={canManage}
              onSelectApp={onSelectApp}
              onSelectPermission={onSelectPermission}
              onCreateGrant={onCreateGrant}
              onRevokeGrant={onRevokeGrant}
              onBan={onBan}
              onUnban={onUnban}
              onSetManager={onSetManager}
            />
          )}
        </div>
      </div>
    </section>
  );
}
