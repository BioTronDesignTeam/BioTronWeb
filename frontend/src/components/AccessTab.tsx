import { useMemo } from 'react';
import type { AppInfo, Grant, Me, Permission } from '../api';
import { divided, mutedText, panel, panelEmpty, panelHeader, panelHeading } from './ui';

/**
 * Held permissions are green chips with a check; the rest are plain outlined
 * chips, so the difference survives without colour. There is nothing to click:
 * access is asked for in a meeting or on Discord and a manager grants it from
 * the Org tab.
 */
function PermissionChip({ label, granted }: { label: string; granted: boolean }) {
  if (granted) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-3 py-1.5 text-sm font-medium text-emerald-700 ring-1 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-800">
        <span aria-hidden="true">✓</span>
        {label}
        <span className="sr-only"> granted</span>
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-full border border-dashed border-slate-300 px-3 py-1.5 text-sm text-slate-500 dark:border-white/15 dark:text-muted">
      {label}
      <span className="sr-only"> not granted</span>
    </span>
  );
}

export default function AccessTab({
  me,
  apps,
  permissions,
  grants,
  fullAccess,
}: {
  me: Me;
  apps: AppInfo[];
  permissions: Permission[];
  grants: Grant[];
  fullAccess: boolean;
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

  const role = me.is_superuser ? 'superuser' : 'manager';

  return (
    <section className={panel}>
      <div className={panelHeader}>
        <h2 className={panelHeading}>Your permissions</h2>
        {fullAccess ? (
          <p className="mt-1 text-sm text-emerald-600 dark:text-emerald-400">
            You have full access to every tool as a {role}.
          </p>
        ) : (
          <p className={`mt-1 ${mutedText}`}>
            To change these, ask a manager in a meeting or on Discord.
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
                <div className="text-base font-medium break-words text-slate-900 dark:text-white">
                  {app.name}
                </div>
                <div className={`mt-1 break-words ${mutedText}`}>{app.description || app.id}</div>
              </div>
              {perms.length === 0 ? (
                <p className="mt-3 text-sm text-slate-400 dark:text-faint">No permissions defined.</p>
              ) : (
                <div className="mt-4 flex flex-wrap gap-2">
                  {perms.map((perm) => (
                    <PermissionChip
                      key={perm.key}
                      label={perm.label}
                      granted={fullAccess || grantSet.has(`${app.id}:${perm.key}`)}
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
