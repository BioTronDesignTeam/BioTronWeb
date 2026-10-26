import { useCallback, useEffect, useMemo, useState } from 'react';
import { SessionLoading, SessionUnavailable, SignInScreen, useSession } from '@biotron/style';
import {
  type Grant,
  type Me,
  banMember,
  createGrant,
  deleteGrant,
  rotateProductDailyKey,
  setManager,
  setUnauthorizedHandler,
  unbanMember,
} from './api';
import AccessTab from './components/AccessTab';
import AppHeader from './components/AppHeader';
import KeysTab from './components/KeysTab';
import OrgTab from './components/OrgTab';
import TabBar, { type Tab } from './components/TabBar';
import { useAccessDirectory } from './hooks/useAccessDirectory';
import { useDailyKeyReveal } from './hooks/useDailyKeyReveal';
import { useMemberGrants, useOrgSelection } from './hooks/useOrgSelection';

export default function App() {
  const { status, operator, expire } = useSession();

  useEffect(() => {
    setUnauthorizedHandler(expire);
  }, [expire]);

  if (status === 'loading') return <SessionLoading />;
  if (status === 'unavailable') return <SessionUnavailable productName="Auth" />;
  if (status === 'anon' || !operator) return <SignInScreen productName="Auth" />;
  // Keyed by person, so nothing one operator loaded outlives a sign-out.
  return <Console key={operator.github_id} me={operator} />;
}

function Console({ me }: { me: Me }) {
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('access');

  const { directory, refresh } = useAccessDirectory(me);
  const reportError = useCallback((message: string) => setError(message), []);
  const keyReveal = useDailyKeyReveal(reportError);
  const { selection, selectMember, selectApp, selectPermission, clearPermission } = useOrgSelection();
  const { grants: memberGrantList, reload: reloadMemberGrants } = useMemberGrants(
    selection.memberId,
    reportError,
  );

  const selectedMember = useMemo(
    () => directory.members.find((m) => m.github_id === selection.memberId) ?? null,
    [directory.members, selection.memberId],
  );

  // Every mutation follows the same shape: clear the last complaint, act, and
  // surface whatever went wrong. Naming it once keeps the handlers below to a
  // single readable line each.
  const runAction = useCallback(async (fallback: string, action: () => Promise<void>) => {
    setError(null);
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : fallback);
    }
  }, []);

  const onBan = (id: number) =>
    void runAction('Ban failed', async () => {
      await banMember(id);
      await refresh();
      await reloadMemberGrants();
    });

  const onUnban = (id: number) =>
    void runAction('Unban failed', async () => {
      await unbanMember(id);
      await refresh();
      await reloadMemberGrants();
    });

  const onSetManager = (id: number, manager: boolean) =>
    void runAction('Update failed', async () => {
      await setManager(id, manager);
      await refresh();
    });

  const onCreateGrant = () => {
    const { memberId, appId, permissionKey } = selection;
    if (!memberId || !appId || !permissionKey) return;
    void runAction('Grant failed', async () => {
      await createGrant(memberId, appId, permissionKey);
      clearPermission();
      await reloadMemberGrants();
    });
  };

  const onRevokeGrant = (grant: Grant) =>
    void runAction('Revoke failed', async () => {
      await deleteGrant(grant.operator_id, grant.app_id, grant.permission_key);
      await reloadMemberGrants();
    });

  const onRotateKey = (appId: string) =>
    void runAction('Rotate failed', async () => {
      await rotateProductDailyKey(appId);
      await refresh();
    });

  if (directory.loading) return <SessionLoading />;

  return (
    <div className="min-h-screen pb-safe">
      <AppHeader />

      <main className="mx-auto w-full max-w-6xl px-page py-6 sm:py-8 lg:py-10">
        <TabBar tab={tab} onSelect={setTab} isStaff={me.is_staff === true} />

        {(error ?? directory.error) && (
          <p className="mb-6 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
            {error ?? directory.error}
          </p>
        )}

        {tab === 'access' && (
          <AccessTab
            me={me}
            apps={directory.apps}
            permissions={directory.permissions}
            grants={directory.grants}
            fullAccess={directory.fullAccess}
          />
        )}

        {tab === 'keys' && me.is_staff && (
          <KeysTab
            dailyKeys={directory.dailyKeys}
            revealedAppIds={keyReveal.revealedAppIds}
            copiedAppId={keyReveal.copiedAppId}
            onToggle={keyReveal.toggle}
            onCopy={(key) => void keyReveal.copy(key)}
            onRotate={onRotateKey}
          />
        )}

        {tab === 'org' && me.is_staff && (
          <OrgTab
            members={directory.members}
            selectedMember={selectedMember}
            selection={selection}
            memberGrantList={memberGrantList}
            apps={directory.apps}
            permissions={directory.permissions}
            canManage={me.is_superuser === true}
            onSelectMember={selectMember}
            onSelectApp={selectApp}
            onSelectPermission={selectPermission}
            onCreateGrant={onCreateGrant}
            onRevokeGrant={onRevokeGrant}
            onBan={onBan}
            onUnban={onUnban}
            onSetManager={onSetManager}
          />
        )}
      </main>
    </div>
  );
}
