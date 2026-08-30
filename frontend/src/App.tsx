import { AuthScreen, Brand, ThemeToggle, UserMenu } from '@biotron/style';
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  type AccessRequest,
  type AppInfo,
  type Grant,
  type Me,
  type OrgMember,
  type ProductDailyKey,
  type Permission,
  approveRequest,
  banMember,
  createGrant,
  createRequest,
  deleteGrant,
  denyRequest,
  getProductDailyKeys,
  getMe,
  listApps,
  listOrgMembers,
  listPermissions,
  loginURL,
  logout,
  memberGrants,
  myGrants,
  myRequests,
  pendingRequests,
  setManager,
  unbanMember,
} from './api';

type Tab = 'access' | 'approvals' | 'keys' | 'org';

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
      className={`rounded-lg px-4 py-2.5 text-sm font-medium transition ${
        active
          ? 'bg-white text-slate-900 shadow dark:bg-slate-100'
          : 'text-slate-600 hover:text-slate-800 dark:text-slate-300 dark:hover:text-slate-100'
      }`}
    >
      {children}
    </button>
  );
}

function Badge({ variant, children }: { variant: 'granted' | 'pending'; children: ReactNode }) {
  const styles =
    variant === 'granted'
      ? 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-800'
      : 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-800';
  return (
    <span className={`rounded-full px-3 py-1.5 text-sm font-medium ring-1 ${styles}`}>{children}</span>
  );
}

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [denied, setDenied] = useState(false);
  const [banned, setBanned] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('access');

  const [apps, setApps] = useState<AppInfo[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [grants, setGrants] = useState<Grant[]>([]);
  const [fullAccess, setFullAccess] = useState(false);
  const [requests, setRequests] = useState<AccessRequest[]>([]);
  const [pending, setPending] = useState<AccessRequest[]>([]);

  const [members, setMembers] = useState<OrgMember[]>([]);
  const [selectedMemberId, setSelectedMemberId] = useState<number | null>(null);
  const [memberGrantList, setMemberGrantList] = useState<Grant[]>([]);
  const [grantAppId, setGrantAppId] = useState('');
  const [grantPermissionKey, setGrantPermissionKey] = useState('');
  const [dailyKeys, setDailyKeys] = useState<ProductDailyKey[]>([]);
  const [revealedKeyIds, setRevealedKeyIds] = useState<Set<string>>(new Set());
  const [copiedKeyId, setCopiedKeyId] = useState<string | null>(null);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('auth') === 'denied') {
      setDenied(true);
      window.history.replaceState({}, '', window.location.pathname);
    }
    if (params.get('auth') === 'banned') {
      setBanned(true);
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const user = await getMe();
      setMe(user);
      const [appList, permList, grantsRes, reqList] = await Promise.all([
        listApps(),
        listPermissions(),
        myGrants(),
        myRequests(),
      ]);
      setApps(appList);
      setPermissions(permList);
      setGrants(grantsRes.grants);
      setFullAccess(grantsRes.full_access);
      setRequests(reqList);
      if (user.is_staff) {
        const [pendingList, memberList] = await Promise.all([pendingRequests(), listOrgMembers()]);
        setPending(pendingList);
        setMembers(memberList);
        try {
          setDailyKeys(await getProductDailyKeys());
        } catch {
          setDailyKeys([]);
        }
      } else {
        setPending([]);
        setMembers([]);
        setDailyKeys([]);
      }
    } catch {
      setMe(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectedMember = useMemo(
    () => members.find((m) => m.github_id === selectedMemberId) ?? null,
    [members, selectedMemberId],
  );

  useEffect(() => {
    if (!selectedMemberId) {
      setMemberGrantList([]);
      return;
    }
    let cancelled = false;
    void memberGrants(selectedMemberId)
      .then((g) => {
        if (!cancelled) setMemberGrantList(g);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Failed to load grants');
      });
    return () => {
      cancelled = true;
    };
  }, [selectedMemberId]);

  const permissionsByApp = useMemo(() => {
    const map = new Map<string, Permission[]>();
    for (const p of permissions) {
      const list = map.get(p.app_id) ?? [];
      list.push(p);
      map.set(p.app_id, list);
    }
    return map;
  }, [permissions]);

  const grantSet = useMemo(() => {
    const s = new Set<string>();
    for (const g of grants) s.add(`${g.app_id}:${g.permission_key}`);
    return s;
  }, [grants]);

  const pendingSet = useMemo(() => {
    const s = new Set<string>();
    for (const r of requests) {
      if (r.status === 'pending') s.add(`${r.app_id}:${r.permission_key}`);
    }
    return s;
  }, [requests]);

  const appNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const a of apps) map.set(a.id, a.name);
    return map;
  }, [apps]);

  const permissionLabel = useCallback(
    (appId: string, key: string) => {
      const p = permissions.find((x) => x.app_id === appId && x.key === key);
      return p?.label ?? key;
    },
    [permissions],
  );

  const grantPermissionsForApp = useMemo(() => {
    if (!grantAppId) return [];
    return permissions.filter((p) => p.app_id === grantAppId);
  }, [permissions, grantAppId]);

  async function onRequest(appId: string, permissionKey: string) {
    setError(null);
    try {
      await createRequest(appId, permissionKey);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Request failed');
    }
  }

  async function onApprove(id: string) {
    setError(null);
    try {
      await approveRequest(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Approve failed');
    }
  }

  async function onDeny(id: string) {
    setError(null);
    try {
      await denyRequest(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Deny failed');
    }
  }

  async function onLogout() {
    await logout();
    setMe(null);
    setDailyKeys([]);
    setRevealedKeyIds(new Set());
  }

  async function onCopyDailyKey(key: ProductDailyKey) {
    try {
      await navigator.clipboard.writeText(key.key);
      setCopiedKeyId(key.app_id);
      window.setTimeout(() => setCopiedKeyId((current) => (current === key.app_id ? null : current)), 2000);
    } catch {
      setError('Could not copy key');
    }
  }

  function toggleDailyKey(appId: string) {
    setRevealedKeyIds((current) => {
      const next = new Set(current);
      if (next.has(appId)) next.delete(appId);
      else next.add(appId);
      return next;
    });
  }

  async function refreshMemberGrants() {
    if (!selectedMemberId) return;
    setMemberGrantList(await memberGrants(selectedMemberId));
  }

  async function onBanMember(id: number) {
    setError(null);
    try {
      await banMember(id);
      await refresh();
      await refreshMemberGrants();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ban failed');
    }
  }

  async function onUnbanMember(id: number) {
    setError(null);
    try {
      await unbanMember(id);
      await refresh();
      await refreshMemberGrants();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unban failed');
    }
  }

  async function onSetManager(id: number, manager: boolean) {
    setError(null);
    try {
      await setManager(id, manager);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Update failed');
    }
  }

  async function onCreateGrant() {
    if (!selectedMemberId || !grantAppId || !grantPermissionKey) return;
    setError(null);
    try {
      await createGrant(selectedMemberId, grantAppId, grantPermissionKey);
      setGrantPermissionKey('');
      await refreshMemberGrants();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Grant failed');
    }
  }

  async function onRevokeGrant(grant: Grant) {
    setError(null);
    try {
      await deleteGrant(grant.operator_id, grant.app_id, grant.permission_key);
      await refreshMemberGrants();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Revoke failed');
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-slate-500 dark:text-slate-400">
        Loading…
      </div>
    );
  }

  if (!me) {
    return (
      <AuthScreen
        productName="BioTron Auth"
        description="Sign in with GitHub to request and manage tool access. Access is limited to BioTronDesignTeam members."
        action={{ label: 'Sign in with GitHub', href: loginURL, icon: 'github' }}
        notices={[
          ...(denied ? [{ content: "Access denied — your GitHub account isn't a BioTronDesignTeam member.", tone: 'error' as const }] : []),
          ...(banned ? [{ content: 'Your account has been banned. Contact an administrator if you believe this is a mistake.', tone: 'error' as const }] : []),
        ]}
      />
    );
  }

  const staffTabs: Tab[] = me.is_staff ? ['access', 'approvals', 'keys', 'org'] : ['access'];

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-[#aedbfc]/20 dark:bg-[#160b6c]/90">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-3 px-4 py-4 sm:px-6 lg:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <Brand compact />
            <div>
              <div className="text-base font-semibold text-[#16033c] dark:text-white">BioTron Auth</div>
              <div className="mt-0.5 text-sm text-[#3050b0] dark:text-[#aedbfc]">
                Request and approve access to team tools
              </div>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <ThemeToggle />
            <UserMenu
              user={{
                name: me.name,
                login: me.login,
                avatarUrl: me.avatar_url,
                detail: `@${me.login}${
                  me.is_guest
                    ? ' · guest'
                    : me.is_superuser
                      ? ' · superuser'
                      : me.is_manager
                        ? ' · manager'
                        : me.is_staff
                          ? ' · staff'
                          : ''
                }`,
              }}
              onLogout={onLogout}
            />
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
        <div
          className={`mb-6 rounded-xl bg-slate-200/80 p-1 ring-1 ring-slate-300 dark:bg-slate-800/80 dark:ring-white/10 ${
            staffTabs.length > 1 ? 'grid gap-1' : 'inline-flex'
          }`}
          style={
            staffTabs.length > 1
              ? { gridTemplateColumns: `repeat(${staffTabs.length}, minmax(0, 1fr))` }
              : undefined
          }
        >
          <TabButton active={tab === 'access'} onClick={() => setTab('access')}>
            My access
          </TabButton>
          {me.is_staff && (
            <TabButton active={tab === 'approvals'} onClick={() => setTab('approvals')}>
              Approvals{pending.length ? ` (${pending.length})` : ''}
            </TabButton>
          )}
          {me.is_staff && (
            <TabButton active={tab === 'keys'} onClick={() => setTab('keys')}>
              Keys
            </TabButton>
          )}
          {me.is_staff && (
            <TabButton active={tab === 'org'} onClick={() => setTab('org')}>
              Org
            </TabButton>
          )}
        </div>

        {error && (
          <p className="mb-6 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
            {error}
          </p>
        )}

        {tab === 'access' && (
          <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-white/10 dark:bg-slate-900">
            <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Tools</h2>
              {fullAccess && (
                <p className="mt-1 text-sm text-emerald-600 dark:text-emerald-400">
                  You have full access to all tools.
                </p>
              )}
              {me.is_guest && (
                <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                  Guest session — expires at Eastern midnight when today&apos;s key rotates.
                </p>
              )}
            </div>
            {apps.length === 0 && (
              <p className="px-4 py-10 text-sm text-slate-500 sm:px-6 dark:text-slate-400">
                No apps registered yet.
              </p>
            )}
            <ul className="divide-y divide-slate-200 dark:divide-white/10">
              {apps.map((app) => {
                const perms = permissionsByApp.get(app.id) ?? [];
                return (
                  <li key={app.id} className="px-4 py-5 sm:px-6">
                    <div className="min-w-0">
                      <div className="text-base font-medium text-slate-900 dark:text-slate-100">{app.name}</div>
                      <div className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                        {app.description || app.id}
                      </div>
                    </div>
                    {perms.length === 0 ? (
                      <p className="mt-3 text-sm text-slate-400 dark:text-slate-500">No permissions defined.</p>
                    ) : (
                      <div className="mt-4 flex flex-wrap gap-2">
                        {perms.map((perm) => {
                          const key = `${app.id}:${perm.key}`;
                          if (fullAccess || grantSet.has(key)) {
                            return (
                              <Badge key={perm.key} variant="granted">
                                {perm.label}
                              </Badge>
                            );
                          }
                          if (pendingSet.has(key)) {
                            return (
                              <Badge key={perm.key} variant="pending">
                                {perm.label} pending
                              </Badge>
                            );
                          }
                          if (me.is_guest) {
                            return (
                              <span
                                key={perm.key}
                                className="rounded-lg border border-slate-200 px-3 py-2 text-sm text-slate-400 dark:border-white/10 dark:text-slate-500"
                              >
                                {perm.label}
                              </span>
                            );
                          }
                          return (
                            <button
                              key={perm.key}
                              type="button"
                              onClick={() => void onRequest(app.id, perm.key)}
                              className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800"
                            >
                              Request {perm.label}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          </section>
        )}

        {tab === 'approvals' && me.is_staff && (
          <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-white/10 dark:bg-slate-900">
            <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Pending requests</h2>
            </div>
            {pending.length === 0 && (
              <p className="px-4 py-10 text-sm text-slate-500 sm:px-6 dark:text-slate-400">
                Nothing waiting for approval.
              </p>
            )}
            <ul className="divide-y divide-slate-200 dark:divide-white/10">
              {pending.map((r) => (
                <li
                  key={r.id}
                  className="flex flex-col gap-4 px-4 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6"
                >
                  <div className="min-w-0">
                    <div className="text-base font-medium text-slate-900 dark:text-slate-100">
                      @{r.requester_login} → {r.app_name} / {r.permission_label}
                    </div>
                    <div className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                      {new Date(r.created_at).toLocaleString()}
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-2 sm:flex">
                    <button
                      type="button"
                      onClick={() => void onApprove(r.id)}
                      className="rounded-lg bg-slate-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      onClick={() => void onDeny(r.id)}
                      className="rounded-lg border border-slate-300 px-4 py-2.5 text-sm font-medium text-slate-700 transition hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800"
                    >
                      Deny
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        )}

        {tab === 'keys' && me.is_staff && (
          <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-white/10 dark:bg-slate-900">
            <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Daily product keys</h2>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                Each enabled product receives its own key. Keys rotate independently at Eastern midnight.
              </p>
            </div>
            {dailyKeys.length === 0 ? (
              <p className="px-4 py-10 text-sm text-slate-500 sm:px-6 dark:text-slate-400">
                No products currently use daily keys.
              </p>
            ) : (
              <ul className="divide-y divide-slate-200 dark:divide-white/10">
                {dailyKeys.map((key) => {
                  const revealed = revealedKeyIds.has(key.app_id);
                  return (
                    <li key={key.app_id} className="flex flex-col gap-4 px-4 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                      <div className="min-w-0">
                        <h3 className="text-base font-medium text-slate-900 dark:text-slate-100">{key.app_name}</h3>
                        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                          Valid for {key.day} in America/Toronto
                        </p>
                        <p
                          className="mt-3 font-mono text-lg tracking-[0.16em] text-slate-900 dark:text-slate-100"
                          aria-label={revealed ? `${key.app_name} daily key ${key.key}` : `${key.app_name} daily key hidden`}
                        >
                          {revealed ? key.key : '••••-••••-••••'}
                        </p>
                      </div>
                      <div className="grid grid-cols-2 gap-2 sm:flex">
                        <button
                          type="button"
                          onClick={() => toggleDailyKey(key.app_id)}
                          className="rounded-lg border border-slate-300 px-4 py-2.5 text-sm font-medium text-slate-700 transition hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800"
                        >
                          {revealed ? 'Hide' : 'Reveal'}
                        </button>
                        <button
                          type="button"
                          onClick={() => void onCopyDailyKey(key)}
                          className="rounded-lg bg-slate-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
                        >
                          {copiedKeyId === key.app_id ? 'Copied' : 'Copy'}
                        </button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        )}

        {tab === 'org' && me.is_staff && (
          <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-white/10 dark:bg-slate-900">
            <div className="grid min-h-[28rem] grid-cols-1 lg:grid-cols-[minmax(0,16rem)_1fr]">
              <div className="border-b border-slate-200 lg:border-b-0 lg:border-r dark:border-white/10">
                <div className="border-b border-slate-200 px-4 py-4 dark:border-white/10">
                  <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Members</h2>
                </div>
                {members.length === 0 ? (
                  <p className="px-4 py-6 text-sm text-slate-500 dark:text-slate-400">No members found.</p>
                ) : (
                  <ul className="max-h-[32rem] overflow-y-auto">
                    {members.map((member) => (
                      <li key={member.github_id}>
                        <button
                          type="button"
                          onClick={() => {
                            setSelectedMemberId(member.github_id);
                            setGrantAppId('');
                            setGrantPermissionKey('');
                          }}
                          className={`flex w-full items-center gap-3 px-4 py-3 text-left transition ${
                            selectedMemberId === member.github_id
                              ? 'bg-slate-100 dark:bg-slate-800'
                              : 'hover:bg-slate-50 dark:hover:bg-slate-800/50'
                          }`}
                        >
                          {member.avatar_url ? (
                            <img
                              src={member.avatar_url}
                              alt=""
                              className="h-8 w-8 shrink-0 rounded-full object-cover ring-1 ring-slate-200 dark:ring-white/10"
                            />
                          ) : (
                            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-200 text-xs font-medium text-slate-600 dark:bg-slate-700 dark:text-slate-300">
                              {member.login.slice(0, 1).toUpperCase()}
                            </div>
                          )}
                          <div className="min-w-0">
                            <div className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">
                              {member.name || member.login}
                            </div>
                            <div className="truncate text-xs text-slate-500 dark:text-slate-400">
                              @{member.login}
                              {member.is_banned ? ' · banned' : ''}
                              {member.is_manager ? ' · manager' : ''}
                              {member.is_superuser ? ' · superuser' : ''}
                            </div>
                          </div>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              <div className="min-w-0">
                {!selectedMember ? (
                  <div className="flex h-full items-center justify-center px-6 py-16 text-sm text-slate-500 dark:text-slate-400">
                    Select a member to manage access.
                  </div>
                ) : (
                  <div className="flex flex-col">
                    <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
                      <div className="flex flex-wrap items-start justify-between gap-4">
                        <div className="min-w-0">
                          <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                            {selectedMember.name || selectedMember.login}
                          </h3>
                          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
                            @{selectedMember.login}
                          </p>
                          {selectedMember.last_login_at && (
                            <p className="mt-1 text-xs text-slate-400 dark:text-slate-500">
                              Last login {new Date(selectedMember.last_login_at).toLocaleString()}
                            </p>
                          )}
                        </div>
                        <div className="flex flex-wrap gap-2">
                          {selectedMember.is_banned ? (
                            <button
                              type="button"
                              onClick={() => void onUnbanMember(selectedMember.github_id)}
                              className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800"
                            >
                              Unban
                            </button>
                          ) : (
                            <button
                              type="button"
                              onClick={() => void onBanMember(selectedMember.github_id)}
                              className="rounded-lg border border-red-300 px-3 py-2 text-sm font-medium text-red-700 transition hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/40"
                            >
                              Ban
                            </button>
                          )}
                          {me.is_superuser && (
                            <button
                              type="button"
                              onClick={() =>
                                void onSetManager(selectedMember.github_id, !selectedMember.is_manager)
                              }
                              className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800"
                            >
                              {selectedMember.is_manager ? 'Remove manager' : 'Make manager'}
                            </button>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10">
                      <h4 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Grants</h4>
                      {memberGrantList.length === 0 ? (
                        <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">No grants.</p>
                      ) : (
                        <ul className="mt-3 divide-y divide-slate-200 dark:divide-white/10">
                          {memberGrantList.map((grant) => (
                            <li
                              key={`${grant.app_id}:${grant.permission_key}`}
                              className="flex flex-wrap items-center justify-between gap-3 py-3"
                            >
                              <div className="min-w-0 text-sm text-slate-700 dark:text-slate-300">
                                <span className="font-medium text-slate-900 dark:text-slate-100">
                                  {appNameById.get(grant.app_id) ?? grant.app_id}
                                </span>
                                {' / '}
                                {permissionLabel(grant.app_id, grant.permission_key)}
                              </div>
                              <button
                                type="button"
                                onClick={() => void onRevokeGrant(grant)}
                                className="rounded-lg border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 transition hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/40"
                              >
                                Revoke
                              </button>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>

                    <div className="px-4 py-4 sm:px-6">
                      <h4 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Grant access</h4>
                      <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end">
                        <label className="flex min-w-0 flex-1 flex-col gap-1">
                          <span className="text-xs font-medium text-slate-500 dark:text-slate-400">App</span>
                          <select
                            value={grantAppId}
                            onChange={(e) => {
                              setGrantAppId(e.target.value);
                              setGrantPermissionKey('');
                            }}
                            className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:border-white/10 dark:bg-slate-800 dark:text-slate-100"
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
                          <span className="text-xs font-medium text-slate-500 dark:text-slate-400">Permission</span>
                          <select
                            value={grantPermissionKey}
                            onChange={(e) => setGrantPermissionKey(e.target.value)}
                            disabled={!grantAppId}
                            className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 disabled:opacity-50 dark:border-white/10 dark:bg-slate-800 dark:text-slate-100"
                          >
                            <option value="">Select permission…</option>
                            {grantPermissionsForApp.map((perm) => (
                              <option key={perm.key} value={perm.key}>
                                {perm.label}
                              </option>
                            ))}
                          </select>
                        </label>
                        <button
                          type="button"
                          disabled={!grantAppId || !grantPermissionKey}
                          onClick={() => void onCreateGrant()}
                          className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:opacity-50 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
                        >
                          Grant
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </section>
        )}
      </main>
    </div>
  );
}
