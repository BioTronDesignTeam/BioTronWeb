import type { AuthStatus, EventOverride, EventPayload, EventSeries, Occurrence, Scope } from './types';

export const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8083').replace(/\/$/, '');
export const AUTH_URL = (import.meta.env.VITE_AUTH_URL || 'http://localhost:8080').replace(/\/$/, '');
export const SITE_URL = (import.meta.env.VITE_SITE_URL || 'http://localhost:5177').replace(/\/$/, '');

async function request<T>(path: string, init: RequestInit = {}, authenticated = false): Promise<T> {
  const mutating = init.method && init.method !== 'GET';
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: authenticated ? 'include' : 'omit',
    headers: {
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...(mutating ? { 'X-Requested-With': 'XMLHttpRequest' } : {}),
      ...init.headers,
    },
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

export const calendarApi = {
  authStatus: () => request<AuthStatus>('/v1/auth/status', {}, true),
  scopes: () => request<Scope[]>('/v1/scopes'),
  occurrences: (from: string, to: string, scopeId?: string) => {
    const query = new URLSearchParams({ from, to });
    if (scopeId) query.set('scope_id', scopeId);
    return request<Occurrence[]>(`/v1/events?${query}`);
  },
  adminScopes: () => request<Scope[]>('/v1/admin/scopes', {}, true),
  adminEvents: () => request<EventSeries[]>('/v1/admin/events', {}, true),
  createScope: (body: { kind: 'PROJECT' | 'SUBTEAM'; name: string; parent_id: string }) =>
    request<Scope>('/v1/admin/scopes', { method: 'POST', body: JSON.stringify(body) }, true),
  renameScope: (id: string, name: string) =>
    request<Scope>(`/v1/admin/scopes/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }, true),
  archiveScope: (id: string) =>
    request<Scope>(`/v1/admin/scopes/${id}/archive`, { method: 'POST', body: '{}' }, true),
  restoreScope: (id: string) =>
    request<Scope>(`/v1/admin/scopes/${id}/restore`, { method: 'POST', body: '{}' }, true),
  deleteScope: (id: string) => request<void>(`/v1/admin/scopes/${id}`, { method: 'DELETE' }, true),
  createEvent: (body: EventPayload) =>
    request<EventSeries>('/v1/admin/events', { method: 'POST', body: JSON.stringify(body) }, true),
  updateEvent: (id: string, body: EventPayload) =>
    request<EventSeries>(`/v1/admin/events/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, true),
  publishEvent: (id: string, expectedSequence: number) =>
    request<EventSeries>(`/v1/admin/events/${id}/publish`, {
      method: 'POST', body: JSON.stringify({ expected_sequence: expectedSequence }),
    }, true),
  cancelEvent: (id: string, expectedSequence: number) =>
    request<EventSeries>(`/v1/admin/events/${id}/cancel`, {
      method: 'POST', body: JSON.stringify({ expected_sequence: expectedSequence }),
    }, true),
  deleteEvent: (id: string) => request<void>(`/v1/admin/events/${id}`, { method: 'DELETE' }, true),
  updateOccurrence: (
    id: string,
    body: { recurrence_id_local: string; state: 'MODIFIED' | 'CANCELLED'; patch: Record<string, string | null>; expected_sequence: number },
  ) => request<EventOverride>(`/v1/admin/events/${id}/occurrences`, {
    method: 'PUT', body: JSON.stringify(body),
  }, true),
  resetOccurrence: (id: string, recurrenceId: string, expectedSequence: number) => {
    const query = new URLSearchParams({ recurrence_id_local: recurrenceId, expected_sequence: String(expectedSequence) });
    return request<void>(`/v1/admin/events/${id}/occurrences?${query}`, { method: 'DELETE' }, true);
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

export function feedURL(scopeId?: string) {
  return scopeId ? `${API_URL}/v1/feeds/scopes/${scopeId}.ics` : `${API_URL}/v1/feeds/all.ics`;
}

export function webcalURL(scopeId?: string) {
  return feedURL(scopeId).replace(/^https?:/, 'webcal:');
}
