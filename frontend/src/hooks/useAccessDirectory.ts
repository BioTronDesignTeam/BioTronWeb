import { useCallback, useEffect, useReducer } from 'react';
import {
  type AppInfo,
  type Grant,
  type Me,
  type OrgMember,
  type Permission,
  type ProductDailyKey,
  getMe,
  getProductDailyKeys,
  listApps,
  listOrgMembers,
  listPermissions,
  myGrants,
} from '../api';

/** Everything one signed-in operator can see, loaded as a single snapshot. */
export type AccessDirectory = {
  loading: boolean;
  me: Me | null;
  apps: AppInfo[];
  permissions: Permission[];
  grants: Grant[];
  fullAccess: boolean;
  members: OrgMember[];
  dailyKeys: ProductDailyKey[];
};

type Snapshot = Omit<AccessDirectory, 'loading'>;

type Action = { type: 'loaded'; snapshot: Snapshot } | { type: 'cleared' };

const signedOut: Snapshot = {
  me: null,
  apps: [],
  permissions: [],
  grants: [],
  fullAccess: false,
  members: [],
  dailyKeys: [],
};

const initial: AccessDirectory = { loading: true, ...signedOut };

// These fields are always fetched and replaced together, so one action
// describes the whole transition rather than a dozen setter calls that could
// leave the screen showing one operator's identity beside another's grants.
function reducer(state: AccessDirectory, action: Action): AccessDirectory {
  switch (action.type) {
    case 'loaded':
      return { loading: false, ...action.snapshot };
    case 'cleared':
      return { ...state, loading: false, me: null, dailyKeys: [] };
  }
}

type StaffSnapshot = Pick<Snapshot, 'members' | 'dailyKeys'>;

/** The org roster and the daily keys only exist for staff. */
async function loadStaffSnapshot(me: Me): Promise<StaffSnapshot> {
  if (!me.is_staff) return { members: [], dailyKeys: [] };
  const [members, dailyKeys] = await Promise.all([
    listOrgMembers(),
    // A product with no key configured must not blank the whole screen.
    getProductDailyKeys().catch((): ProductDailyKey[] => []),
  ]);
  return { members, dailyKeys };
}

async function loadSnapshot(): Promise<Snapshot> {
  // Identity first: it decides which of the following requests are worth
  // making at all, and a rejection here is what "signed out" looks like.
  const me = await getMe();
  const [apps, permissions, grantsResponse, staff] = await Promise.all([
    listApps(),
    listPermissions(),
    myGrants(),
    loadStaffSnapshot(me),
  ]);

  return {
    me,
    apps,
    permissions,
    grants: grantsResponse.grants,
    fullAccess: grantsResponse.full_access,
    ...staff,
  };
}

export function useAccessDirectory() {
  const [directory, dispatch] = useReducer(reducer, initial);

  const refresh = useCallback(async () => {
    try {
      dispatch({ type: 'loaded', snapshot: await loadSnapshot() });
    } catch {
      dispatch({ type: 'cleared' });
    }
  }, []);

  const clear = useCallback(() => dispatch({ type: 'cleared' }), []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { directory, refresh, clear };
}
