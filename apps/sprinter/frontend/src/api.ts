import type { Automation, AutomationPayload, AuthStatus, Guard, GuardSubject, Scope } from './types';

export const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8084').replace(/\/$/, '');
export const AUTH_URL = (import.meta.env.VITE_AUTH_URL || 'http://localhost:8080').replace(/\/$/, '');
export const CALENDAR_URL = (import.meta.env.VITE_CALENDAR_URL || 'http://localhost:8083').replace(/\/$/, '');

async function request<T>(path: string, init: RequestInit = {}, authenticated = false): Promise<T> {
  const mutating = init.method && init.method !== 'GET';
  // init.headers may be a Headers instance, a list of pairs, or a record;
  // Headers accepts all three, where spreading would not.
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  if (mutating) headers.set('X-Requested-With', 'XMLHttpRequest');
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: authenticated ? 'include' : 'omit',
    headers,
  });
  if (!response.ok) {
    let message = `Request failed (${response.status})`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // Preserve the status fallback when the response is not JSON.
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const sprinterApi = {
  authStatus: () => request<AuthStatus>('/v1/auth/status', {}, true),
  guards: () => request<Guard[]>('/v1/admin/guards', {}, true),
  saveGuard: (subject: GuardSubject, body: { guild_id: string; role_ids: string[]; channel_ids: string[] }) =>
    request<Guard>(`/v1/admin/guards/${subject}`, { method: 'PUT', body: JSON.stringify(body) }, true),
  clearGuard: (subject: GuardSubject) =>
    request<void>(`/v1/admin/guards/${subject}`, { method: 'DELETE' }, true),
  automations: () => request<Automation[]>('/v1/admin/automations', {}, true),
  createAutomation: (body: AutomationPayload) =>
    request<Automation>('/v1/admin/automations', { method: 'POST', body: JSON.stringify(body) }, true),
  updateAutomation: (id: string, body: Partial<AutomationPayload>) =>
    request<Automation>(`/v1/admin/automations/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, true),
  deleteAutomation: (id: string) =>
    request<void>(`/v1/admin/automations/${id}`, { method: 'DELETE' }, true),
  // The Calendar scopes list is public and unauthenticated, and lives on a
  // different origin/service entirely, so it bypasses the Sprinter API
  // helper above rather than being folded into `request`.
  scopes: async (): Promise<Scope[]> => {
    const response = await fetch(`${CALENDAR_URL}/v1/scopes`, { credentials: 'omit' });
    if (!response.ok) {
      let message = `Request failed (${response.status})`;
      try {
        const body = (await response.json()) as { error?: string };
        if (body.error) message = body.error;
      } catch {
        // Preserve the status fallback when the response is not JSON.
      }
      throw new Error(message);
    }
    return response.json() as Promise<Scope[]>;
  },
};

export function login() {
  window.location.href = `${AUTH_URL}/auth/github/login?redirect=${encodeURIComponent(window.location.origin)}`;
}

export async function logout() {
  const response = await fetch(`${AUTH_URL}/auth/logout`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'X-Requested-With': 'XMLHttpRequest' },
  });
  if (!response.ok) throw new Error(`Sign out failed (${response.status})`);
}
