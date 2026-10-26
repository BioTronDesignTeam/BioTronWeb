import { friendlyError, type SessionOperator } from '@biotron/style';

export const API_BASE = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080';

// Told when a request answers 401, so the session is re-checked and the page
// says it expired instead of showing the raw refusal.
let onUnauthorized: () => void = () => undefined;
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler;
}

async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.method && init.method !== 'GET') {
    headers.set('X-Requested-With', 'XMLHttpRequest');
  }
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: 'include',
    headers,
  });
  if (!res.ok) {
    if (res.status === 401) onUnauthorized();
    let message = '';
    try {
      const data = await res.json();
      if (data?.error) message = data.error;
    } catch {
      /* ignore */
    }
    throw new Error(friendlyError(res.status, message));
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

/** Who is signed in, as Auth's `GET /auth/me` sends it. */
export type Me = SessionOperator;

export type ProductDailyKey = {
  app_id: string;
  app_name: string;
  day: string;
  key: string;
};

export type AppInfo = {
  id: string;
  name: string;
  description: string;
  daily_key_enabled: boolean;
};

export type Permission = {
  app_id: string;
  key: string;
  label: string;
  description: string;
};

export type Grant = {
  operator_id: number;
  app_id: string;
  permission_key: string;
  created_at: string;
};

export type GrantsResponse = {
  grants: Grant[];
  full_access: boolean;
};

export type OrgMember = {
  github_id: number;
  login: string;
  name: string;
  avatar_url: string;
  is_superuser: boolean;
  is_manager: boolean;
  is_banned: boolean;
  last_login_at: string;
};

export const getProductDailyKeys = () => api<ProductDailyKey[]>('/auth/guest-keys');
export const rotateProductDailyKey = (appId: string) =>
  api<ProductDailyKey>(`/auth/guest-keys/${encodeURIComponent(appId)}/rotate`, { method: 'POST' });
export const listApps = () => api<AppInfo[]>('/apps');
export const listPermissions = () => api<Permission[]>('/permissions');
export const myGrants = () => api<GrantsResponse>('/me/grants');

export const listOrgMembers = () => api<OrgMember[]>('/org/members');
export const memberGrants = (id: number) => api<Grant[]>(`/org/members/${id}/grants`);
export const banMember = (id: number) => api<void>(`/org/members/${id}/ban`, { method: 'POST' });
export const unbanMember = (id: number) => api<void>(`/org/members/${id}/unban`, { method: 'POST' });
export const setManager = (id: number, manager: boolean) =>
  api<void>(`/org/members/${id}/manager`, {
    method: 'PATCH',
    body: JSON.stringify({ manager }),
  });
export const createGrant = (operator_id: number, app_id: string, permission_key: string) =>
  api<void>('/grants', {
    method: 'POST',
    body: JSON.stringify({ operator_id, app_id, permission_key }),
  });
export const deleteGrant = (operator_id: number, app_id: string, permission_key: string) =>
  api<void>('/grants', {
    method: 'DELETE',
    body: JSON.stringify({ operator_id, app_id, permission_key }),
  });
