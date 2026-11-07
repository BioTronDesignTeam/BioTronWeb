import { useCallback, useEffect, useReducer, useState } from 'react';
import { type Grant, memberGrants } from '../api';

/**
 * Picking a member, an app, and a permission is one cascading choice: a new
 * member invalidates both dropdowns and a new app invalidates the permission.
 * Keeping the three in a reducer means that cascade is described once instead
 * of being re-implemented at every call site that changes part of it.
 */
export type OrgSelection = {
  memberId: number | null;
  appId: string;
  permissionKey: string;
};

type Action =
  | { type: 'select-member'; memberId: number }
  | { type: 'select-app'; appId: string }
  | { type: 'select-permission'; permissionKey: string }
  | { type: 'clear-permission' };

const noSelection: OrgSelection = { memberId: null, appId: '', permissionKey: '' };

function reducer(state: OrgSelection, action: Action): OrgSelection {
  switch (action.type) {
    case 'select-member':
      return { memberId: action.memberId, appId: '', permissionKey: '' };
    case 'select-app':
      return { ...state, appId: action.appId, permissionKey: '' };
    case 'select-permission':
      return { ...state, permissionKey: action.permissionKey };
    case 'clear-permission':
      return { ...state, permissionKey: '' };
  }
}

export function useOrgSelection() {
  const [selection, dispatch] = useReducer(reducer, noSelection);

  return {
    selection,
    selectMember: useCallback((memberId: number) => dispatch({ type: 'select-member', memberId }), []),
    selectApp: useCallback((appId: string) => dispatch({ type: 'select-app', appId }), []),
    selectPermission: useCallback(
      (permissionKey: string) => dispatch({ type: 'select-permission', permissionKey }),
      [],
    ),
    clearPermission: useCallback(() => dispatch({ type: 'clear-permission' }), []),
  };
}

const noGrants: Grant[] = [];

/** Grants of the currently selected member, reloaded whenever the choice moves. */
export function useMemberGrants(memberId: number | null, onError: (message: string) => void) {
  // Grants are kept with the member they belong to, so a change of member
  // shows none until that member's grants land, never the last member's.
  const [loaded, setLoaded] = useState<{ memberId: number; grants: Grant[] } | null>(null);
  const grants = memberId && loaded?.memberId === memberId ? loaded.grants : noGrants;

  useEffect(() => {
    if (!memberId) return;
    let cancelled = false;
    void memberGrants(memberId)
      .then((next) => {
        if (!cancelled) setLoaded({ memberId, grants: next });
      })
      .catch((e) => {
        if (!cancelled) onError(e instanceof Error ? e.message : 'Failed to load grants');
      });
    return () => {
      cancelled = true;
    };
  }, [memberId, onError]);

  const reload = useCallback(async () => {
    if (!memberId) return;
    setLoaded({ memberId, grants: await memberGrants(memberId) });
  }, [memberId]);

  return { grants, reload };
}
