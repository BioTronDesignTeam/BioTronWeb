import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  SessionContext,
  readNotice,
  stripNotice,
  type SessionNotice,
  type SessionOperator,
  type SessionStatus,
  type SessionValue,
} from './session';
import { authStrings } from './strings';

// Re-check this often while the tab is open, so a guest session that ends at
// Eastern midnight, a ban, or a revoked permission shows without a reload.
const REVALIDATE_MS = 10 * 60 * 1000;
// While Auth is unreachable, try again this often.
const UNAVAILABLE_RETRY_MS = 15 * 1000;
// Focus and visibility events come in bursts; one check covers them all.
const FOCUS_THROTTLE_MS = 30 * 1000;
const ATTEMPTS = 3;

export type SessionProviderProps = {
  /** Auth's base URL, such as `https://auth.example.org`. */
  authUrl: string;
  /**
   * The app id in Auth. A guest session only counts for its own app, so leave
   * it out only on Auth's own pages, where guests are always refused.
   */
  appId?: string;
  /** Permission keys of `appId` to check after sign-in. */
  permissions?: readonly string[];
  children: ReactNode;
};

type State = {
  status: SessionStatus;
  operator: SessionOperator | null;
  permissions: Record<string, boolean>;
  notice: SessionNotice | null;
};

type Check =
  | { kind: 'authed'; operator: SessionOperator; permissions: Record<string, boolean> }
  | { kind: 'anon' }
  | { kind: 'error' };

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function checkSession(authUrl: string, appId: string | undefined, permissions: readonly string[]): Promise<Check> {
  try {
    const query = appId ? `?app=${encodeURIComponent(appId)}` : '';
    // credentials: 'include' so the cross-origin session cookie is sent.
    const me = await fetch(`${authUrl}/auth/me${query}`, { credentials: 'include' });
    // Only an explicit 401 or 403 means "not signed in" (403 is a guest of
    // another app). Anything else is an outage, not a sign-out.
    if (me.status === 401 || me.status === 403) return { kind: 'anon' };
    if (!me.ok) return { kind: 'error' };
    const operator = (await me.json()) as SessionOperator;

    const granted: Record<string, boolean> = {};
    if (appId) {
      const answers = await Promise.all(permissions.map(async (permission) => {
        const query = new URLSearchParams({ app: appId, permission });
        const res = await fetch(`${authUrl}/v1/check?${query}`, { credentials: 'include' });
        if (res.status === 401) return 'anon' as const;
        if (!res.ok) return 'error' as const;
        const body = (await res.json()) as { allowed?: boolean };
        granted[permission] = body.allowed === true;
        return 'ok' as const;
      }));
      if (answers.includes('anon')) return { kind: 'anon' };
      if (answers.includes('error')) return { kind: 'error' };
    }
    return { kind: 'authed', operator, permissions: granted };
  } catch {
    // A network or CORS failure is transient.
    return { kind: 'error' };
  }
}

/**
 * One signed-in state for a BioTron tool, read from Auth directly. It retries
 * before it gives up, never drops a live session over a network blip, re-checks
 * on focus and every ten minutes, and tells an Auth outage (`unavailable`)
 * apart from being signed out.
 */
export function SessionProvider({ authUrl, appId, permissions = [], children }: SessionProviderProps) {
  const base = authUrl.replace(/\/$/, '');
  const permissionKey = permissions.join(',');
  const [state, setState] = useState<State>(() => ({
    status: 'loading',
    operator: null,
    permissions: {},
    // Read during the first render, so the sign-in screen never paints a
    // frame without its explanation.
    notice: readNotice(),
  }));
  const generation = useRef(0);
  const lastCheck = useRef(0);

  useEffect(() => {
    stripNotice();
  }, []);

  const refresh = useCallback(async () => {
    const run = ++generation.current;
    lastCheck.current = Date.now();
    const wanted = permissionKey ? permissionKey.split(',') : [];
    for (let attempt = 0; attempt < ATTEMPTS; attempt++) {
      const result = await checkSession(base, appId, wanted);
      // A newer check started meanwhile; it owns the state.
      if (run !== generation.current) return;
      if (result.kind === 'authed') {
        setState((prev) => ({
          status: 'authed',
          operator: result.operator,
          permissions: result.permissions,
          notice: prev.notice === 'expired' ? null : prev.notice,
        }));
        return;
      }
      if (result.kind === 'anon') {
        setState((prev) => ({
          status: 'anon',
          operator: null,
          permissions: {},
          // A session that was live a moment ago and is now refused expired.
          notice: prev.status === 'authed' ? 'expired' : prev.notice,
        }));
        return;
      }
      if (attempt < ATTEMPTS - 1) await wait(500 * (attempt + 1));
    }
    if (run !== generation.current) return;
    // Keep a live session through an outage; otherwise say Auth is down.
    setState((prev) => (prev.status === 'authed' ? prev : { ...prev, status: 'unavailable' }));
  }, [base, appId, permissionKey]);

  useEffect(() => {
    // Checking with Auth is the external system this effect syncs with.
    // oxlint-disable-next-line react/set-state-in-effect
    void refresh();
    const revalidate = () => {
      if (document.visibilityState !== 'visible') return;
      if (Date.now() - lastCheck.current < FOCUS_THROTTLE_MS) return;
      void refresh();
    };
    document.addEventListener('visibilitychange', revalidate);
    window.addEventListener('focus', revalidate);
    const interval = window.setInterval(() => void refresh(), REVALIDATE_MS);
    return () => {
      document.removeEventListener('visibilitychange', revalidate);
      window.removeEventListener('focus', revalidate);
      window.clearInterval(interval);
    };
  }, [refresh]);

  const unavailable = state.status === 'unavailable';
  useEffect(() => {
    if (!unavailable) return;
    const retry = window.setInterval(() => void refresh(), UNAVAILABLE_RETRY_MS);
    return () => window.clearInterval(retry);
  }, [unavailable, refresh]);

  const login = useCallback(() => {
    window.location.href = `${base}/auth/github/login?redirect=${encodeURIComponent(window.location.origin)}`;
  }, [base]);

  const loginGuest = useCallback(async (key: string) => {
    if (!appId) return false;
    try {
      const res = await fetch(`${base}/auth/guest`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ app_id: appId, key }),
      });
      if (!res.ok) return false;
      await refresh();
      return true;
    } catch {
      return false;
    }
  }, [appId, base, refresh]);

  const logout = useCallback(async () => {
    let res: Response;
    try {
      res = await fetch(`${base}/auth/logout`, {
        method: 'POST',
        credentials: 'include',
        // The custom header forces a CORS preflight that a cross-site page
        // cannot pass, and Auth requires it: together they block sign-out CSRF.
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      });
    } catch {
      throw new Error(authStrings.signOutFailed);
    }
    // 401 means the session had already ended, which is what was asked for.
    // Anything else keeps the state: the cookie may still be live.
    if (!res.ok && res.status !== 401) throw new Error(authStrings.signOutFailed);
    generation.current++;
    setState({ status: 'anon', operator: null, permissions: {}, notice: null });
  }, [base]);

  const dismissNotice = useCallback(() => setState((prev) => ({ ...prev, notice: null })), []);
  const expire = useCallback(() => void refresh(), [refresh]);

  const value = useMemo<SessionValue>(() => {
    const granted = state.permissions;
    return {
      status: state.status,
      operator: state.operator,
      permissions: granted,
      can: (permission) => {
        const list = typeof permission === 'string' ? [permission] : permission;
        return list.some((key) => granted[key] === true);
      },
      notice: state.notice,
      dismissNotice,
      login,
      loginGuest,
      logout,
      refresh,
      expire,
    };
  }, [state, dismissNotice, login, loginGuest, logout, refresh, expire]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}
