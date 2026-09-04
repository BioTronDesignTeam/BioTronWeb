import { useMemo } from 'react';
import type { AppInfo, Grant, OrgMember, Permission } from '../api';
import type { OrgSelection } from '../hooks/useOrgSelection';
import {
  compactDangerButton,
  compactPrimaryButton,
  compactSecondaryButton,
  dangerButton,
  divided,
  mutedText,
  selectControl,
} from './ui';

const sectionHeading = 'text-sm font-semibold text-slate-900 dark:text-slate-100';
const fieldLabel = 'text-xs font-medium text-slate-500 dark:text-slate-400';

function MemberActions({
  member,
  canManage,
  onBan,
  onUnban,
  onSetManager,
}: {
  member: OrgMember;
  canManage: boolean;
  onBan: (id: number) => void;
  onUnban: (id: number) => void;
  onSetManager: (id: number, manager: boolean) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {member.is_banned ? (
        <button
          type="button"
          onClick={() => onUnban(member.github_id)}
          className={compactSecondaryButton}
        >
          Unban
        </button>
      ) : (
        <button type="button" onClick={() => onBan(member.github_id)} className={dangerButton}>
          Ban
        </button>
      )}
      {canManage && (
        <button
          type="button"
          onClick={() => onSetManager(member.github_id, !member.is_manager)}
          className={compactSecondaryButton}
        >
          {member.is_manager ? 'Remove manager' : 'Make manager'}
        </button>
      )}
    </div>
  );
}

function GrantList({
  grants,
  describe,
  onRevoke,
}: {
  grants: Grant[];
  describe: (grant: Grant) => { app: string; permission: string };
  onRevoke: (grant: Grant) => void;
}) {
  if (grants.length === 0) {
    return <p className={`mt-2 ${mutedText}`}>No grants.</p>;
  }
  return (
    <ul className={`mt-3 ${divided}`}>
      {grants.map((grant) => {
        const label = describe(grant);
        return (
          <li
            key={`${grant.app_id}:${grant.permission_key}`}
            className="flex flex-wrap items-center justify-between gap-3 py-3"
          >
            <div className="min-w-0 text-sm break-words text-slate-700 dark:text-slate-300">
              <span className="font-medium text-slate-900 dark:text-slate-100">{label.app}</span>
              {' / '}
              {label.permission}
            </div>
            <button type="button" onClick={() => onRevoke(grant)} className={compactDangerButton}>
              Revoke
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function GrantForm({
  apps,
  permissions,
  selection,
  onSelectApp,
  onSelectPermission,
  onSubmit,
}: {
  apps: AppInfo[];
  permissions: Permission[];
  selection: OrgSelection;
  onSelectApp: (appId: string) => void;
  onSelectPermission: (permissionKey: string) => void;
  onSubmit: () => void;
}) {
  const permissionsForApp = useMemo(
    () => (selection.appId ? permissions.filter((p) => p.app_id === selection.appId) : []),
    [permissions, selection.appId],
  );

  return (
    <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end">
      <label className="flex min-w-0 flex-1 flex-col gap-1">
        <span className={fieldLabel}>App</span>
        <select
          value={selection.appId}
          onChange={(e) => onSelectApp(e.target.value)}
          className={selectControl}
        >
          <option value="">Select app…</option>
          {apps.map((app) => (
            <option key={app.id} value={app.id}>
              {app.name}
            </option>
          ))}
        </select>
      </label>
      <label className="flex min-w-0 flex-1 flex-col gap-1">
        <span className={fieldLabel}>Permission</span>
        <select
          value={selection.permissionKey}
          onChange={(e) => onSelectPermission(e.target.value)}
          disabled={!selection.appId}
          className={selectControl}
        >
          <option value="">Select permission…</option>
          {permissionsForApp.map((permission) => (
            <option key={permission.key} value={permission.key}>
              {permission.label}
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        disabled={!selection.appId || !selection.permissionKey}
        onClick={onSubmit}
        className={compactPrimaryButton}
      >
        Grant
      </button>
    </div>
  );
}

export default function OrgMemberDetail({
  member,
  apps,
  permissions,
  grants,
  selection,
  canManage,
  onSelectApp,
  onSelectPermission,
  onCreateGrant,
  onRevokeGrant,
  onBan,
  onUnban,
  onSetManager,
}: {
  member: OrgMember;
  apps: AppInfo[];
  permissions: Permission[];
  grants: Grant[];
  selection: OrgSelection;
  canManage: boolean;
  onSelectApp: (appId: string) => void;
  onSelectPermission: (permissionKey: string) => void;
  onCreateGrant: () => void;
  onRevokeGrant: (grant: Grant) => void;
  onBan: (id: number) => void;
  onUnban: (id: number) => void;
  onSetManager: (id: number, manager: boolean) => void;
}) {
  const appNameById = useMemo(() => new Map(apps.map((app) => [app.id, app.name])), [apps]);

  const describe = (grant: Grant) => ({
    app: appNameById.get(grant.app_id) ?? grant.app_id,
    permission:
      permissions.find((p) => p.app_id === grant.app_id && p.key === grant.permission_key)?.label ??
      grant.permission_key,
  });

  return (
    <div className="flex flex-col">
      <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            <h3 className="text-base font-semibold break-words text-slate-900 dark:text-slate-100">
              {member.name || member.login}
            </h3>
            <p className={`mt-0.5 break-words ${mutedText}`}>@{member.login}</p>
            {member.last_login_at && (
              <p className="mt-1 text-xs text-slate-400 dark:text-slate-500">
                Last login {new Date(member.last_login_at).toLocaleString()}
              </p>
            )}
          </div>
          <MemberActions
            member={member}
            canManage={canManage}
            onBan={onBan}
            onUnban={onUnban}
            onSetManager={onSetManager}
          />
        </div>
      </div>

      <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
        <h4 className={sectionHeading}>Grants</h4>
        <GrantList grants={grants} describe={describe} onRevoke={onRevokeGrant} />
      </div>

      <div className="px-4 py-4 sm:px-6">
        <h4 className={sectionHeading}>Grant access</h4>
        <GrantForm
          apps={apps}
          permissions={permissions}
          selection={selection}
          onSelectApp={onSelectApp}
          onSelectPermission={onSelectPermission}
          onSubmit={onCreateGrant}
        />
      </div>
    </div>
  );
}
