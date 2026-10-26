import { createContext, useContext } from 'react';

/** Who is signed in, exactly as Auth's `GET /auth/me` sends it. */
export type SessionOperator = {
  github_id: number;
  login: string;
  name: string;
  avatar_url: string;
  is_superuser?: boolean;
  is_manager?: boolean;
  is_staff?: boolean;
  is_guest?: boolean;
  guest_app_id?: string;
};

/**
 * - `loading`: the first check has not finished.
 * - `anon`: Auth says there is no usable session for this app.
 * - `authed`: signed in; `operator` and `permissions` are set.
 * - `unavailable`: Auth could not be reached and nobody was signed in before.
 *   It is not "signed out", and the provider keeps retrying.
 */
export type SessionStatus = 'loading' | 'anon' | 'authed' | 'unavailable';

/**
 * Why the person is looking at a signed-out page: Auth's callback sends
 * `?auth=denied` (not an org member) and `?auth=banned`; `expired` is set when
 * a session that was live is refused.
 */
export type SessionNotice = 'denied' | 'banned' | 'expired';

export type SessionValue = {
  status: SessionStatus;
  operator: SessionOperator | null;
  /** The permissions the provider was asked to check, by key. */
  permissions: Readonly<Record<string, boolean>>;
  /** Whether the session holds a permission; any of them, given a list. */
  can: (permission: string | readonly string[]) => boolean;
  notice: SessionNotice | null;
  dismissNotice: () => void;
  /** Sends the browser to GitHub through Auth, then back here. */
  login: () => void;
  /** Resolves true when Auth accepted the daily guest key. Needs `appId`. */
  loginGuest: (key: string) => Promise<boolean>;
  /** Throws with a shared message when Auth did not end the session. */
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  /** Call when an API answered 401: re-checks, and says the session expired. */
  expire: () => void;
};

export const SessionContext = createContext<SessionValue | null>(null);

export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside a SessionProvider');
  return value;
}

const noticeParam = 'auth';

/** Reads `?auth=denied|banned` from the current address. */
export function readNotice(): SessionNotice | null {
  const value = new URLSearchParams(window.location.search).get(noticeParam);
  return value === 'denied' || value === 'banned' ? value : null;
}

/** Removes only `?auth=` so a reload does not bring a stale notice back. */
export function stripNotice() {
  const url = new URL(window.location.href);
  if (!url.searchParams.has(noticeParam)) return;
  url.searchParams.delete(noticeParam);
  window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
}
