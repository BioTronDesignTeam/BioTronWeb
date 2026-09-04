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
  return (
    <section className={panel}>
      <div className="grid min-h-[28rem] grid-cols-1 lg:grid-cols-[minmax(0,16rem)_1fr]">
        <OrgMemberList
          members={members}
          selectedMemberId={selection.memberId}
          onSelect={onSelectMember}
        />
        <div className="min-w-0">
          {!selectedMember ? (
            <div className="flex h-full items-center justify-center px-6 py-16 text-sm text-slate-500 dark:text-slate-400">
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
