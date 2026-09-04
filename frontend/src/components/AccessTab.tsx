import { useMemo, type ReactNode } from 'react';
import type { AccessRequest, AppInfo, Grant, Me, Permission } from '../api';
import { compactSecondaryButton, divided, mutedText, panel, panelEmpty, panelHeader, panelHeading } from './ui';

function Badge({ variant, children }: { variant: 'granted' | 'pending'; children: ReactNode }) {
  const styles =
    variant === 'granted'
      ? 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-800'
      : 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800';
  return (
    <span
      className={`inline-flex items-center rounded-full px-3 py-1.5 text-sm font-medium ring-1 ${styles}`}
    >
      {children}
    </span>
  );
}

function PermissionControl({
  permission,
  state,
  onRequest,
}: {
  permission: Permission;
  state: 'granted' | 'pending' | 'locked' | 'requestable';
  onRequest: () => void;
}) {
  if (state === 'granted') return <Badge variant="granted">{permission.label}</Badge>;
  if (state === 'pending') return <Badge variant="pending">{permission.label} pending</Badge>;
  if (state === 'locked') {
    return (
      <span className="inline-flex min-h-11 items-center rounded-lg border border-slate-200 px-3 py-2 text-sm text-slate-400 sm:min-h-0 dark:border-white/10 dark:text-slate-500">
        {permission.label}
      </span>
    );
  }
  return (
    <button type="button" onClick={onRequest} className={compactSecondaryButton}>
      Request {permission.label}
    </button>
  );
}

export default function AccessTab({
  me,
  apps,
  permissions,
  grants,
  requests,
  fullAccess,
  onRequest,
}: {
  me: Me;
  apps: AppInfo[];
  permissions: Permission[];
  grants: Grant[];
  requests: AccessRequest[];
  fullAccess: boolean;
  onRequest: (appId: string, permissionKey: string) => void;
}) {
  const permissionsByApp = useMemo(() => {
    const map = new Map<string, Permission[]>();
    for (const permission of permissions) {
      const list = map.get(permission.app_id) ?? [];
      list.push(permission);
      map.set(permission.app_id, list);
    }
    return map;
  }, [permissions]);

  const grantSet = useMemo(
    () => new Set(grants.map((grant) => `${grant.app_id}:${grant.permission_key}`)),
    [grants],
  );

  const pendingSet = useMemo(() => {
    const keys = new Set<string>();
    for (const request of requests) {
      if (request.status === 'pending') keys.add(`${request.app_id}:${request.permission_key}`);
    }
    return keys;
  }, [requests]);

  function stateOf(appId: string, permissionKey: string) {
    const key = `${appId}:${permissionKey}`;
    if (fullAccess || grantSet.has(key)) return 'granted' as const;
    if (pendingSet.has(key)) return 'pending' as const;
    return me.is_guest ? ('locked' as const) : ('requestable' as const);
  }

  return (
    <section className={panel}>
      <div className={panelHeader}>
        <h2 className={panelHeading}>Tools</h2>
        {fullAccess && (
          <p className="mt-1 text-sm text-emerald-600 dark:text-emerald-400">
            You have full access to all tools.
          </p>
        )}
        {me.is_guest && (
          <p className={`mt-1 ${mutedText}`}>
            Guest session — expires at Eastern midnight when today&apos;s key rotates.
          </p>
        )}
      </div>
      {apps.length === 0 && <p className={panelEmpty}>No apps registered yet.</p>}
      <ul className={divided}>
        {apps.map((app) => {
          const perms = permissionsByApp.get(app.id) ?? [];
          return (
            <li key={app.id} className="px-4 py-5 sm:px-6">
              <div className="min-w-0">
                <div className="text-base font-medium break-words text-slate-900 dark:text-slate-100">
                  {app.name}
                </div>
                <div className={`mt-1 break-words ${mutedText}`}>{app.description || app.id}</div>
              </div>
              {perms.length === 0 ? (
                <p className="mt-3 text-sm text-slate-400 dark:text-slate-500">No permissions defined.</p>
              ) : (
                <div className="mt-4 flex flex-wrap gap-2">
                  {perms.map((perm) => (
                    <PermissionControl
                      key={perm.key}
                      permission={perm}
                      state={stateOf(app.id, perm.key)}
                      onRequest={() => onRequest(app.id, perm.key)}
                    />
                  ))}
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
