import { useCallback, useEffect, useReducer } from 'react';
import {
  type AppInfo,
  type Grant,
  type Me,
  type OrgMember,
  type Permission,
  type ProductDailyKey,
  getProductDailyKeys,
  listApps,
  listOrgMembers,
  listPermissions,
  myGrants,
} from '../api';

/** Everything one signed-in operator can see, loaded as a single snapshot. */
export type AccessDirectory = {
  loading: boolean;
  /** Why the snapshot could not be loaded, in words for the page. */
  error: string | null;
  apps: AppInfo[];
  permissions: Permission[];
  grants: Grant[];
  fullAccess: boolean;
  members: OrgMember[];
  dailyKeys: ProductDailyKey[];
};

type Snapshot = Omit<AccessDirectory, 'loading' | 'error'>;

type Action = { type: 'loaded'; snapshot: Snapshot } | { type: 'failed'; error: string };

const empty: Snapshot = {
  apps: [],
  permissions: [],
  grants: [],
  fullAccess: false,
  members: [],
  dailyKeys: [],
};

const initial: AccessDirectory = { loading: true, error: null, ...empty };

// These fields are always fetched and replaced together, so one action
// describes the whole transition rather than a dozen setter calls that could
// leave the screen showing one operator's grants beside another's roster.
function reducer(state: AccessDirectory, action: Action): AccessDirectory {
  switch (action.type) {
    case 'loaded':
      return { loading: false, error: null, ...action.snapshot };
    case 'failed':
      return { ...state, loading: false, error: action.error };
  }
}

type StaffSnapshot = Pick<Snapshot, 'members' | 'dailyKeys'>;

/** The org roster and the daily guest keys only exist for staff. */
async function loadStaffSnapshot(isStaff: boolean): Promise<StaffSnapshot> {
  if (!isStaff) return { members: [], dailyKeys: [] };
  const [members, dailyKeys] = await Promise.all([
    listOrgMembers(),
    // A tool with no key configured must not blank the whole screen.
    getProductDailyKeys().catch((): ProductDailyKey[] => []),
  ]);
  return { members, dailyKeys };
}

async function loadSnapshot(isStaff: boolean): Promise<Snapshot> {
  const [apps, permissions, grantsResponse, staff] = await Promise.all([
    listApps(),
    listPermissions(),
    myGrants(),
    loadStaffSnapshot(isStaff),
  ]);

  return {
    apps,
    permissions,
    grants: grantsResponse.grants,
    fullAccess: grantsResponse.full_access,
    ...staff,
  };
}

/** The directory for the signed-in operator; the session says who that is. */
export function useAccessDirectory(me: Me) {
  const [directory, dispatch] = useReducer(reducer, initial);
  // A session re-check hands over a new object for the same person; only a
  // change of role is worth reloading for. App remounts this for a new person.
  const isStaff = me.is_staff === true;

  const refresh = useCallback(async () => {
    try {
      dispatch({ type: 'loaded', snapshot: await loadSnapshot(isStaff) });
    } catch (e) {
      dispatch({ type: 'failed', error: e instanceof Error ? e.message : 'Could not load your permissions.' });
    }
  }, [isStaff]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { directory, refresh };
}
