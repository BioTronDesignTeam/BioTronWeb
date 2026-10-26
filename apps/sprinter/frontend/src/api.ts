import { friendlyError } from '@biotron/style';
import type { Automation, AutomationPayload, Guard, GuardSubject, Scope, SprinterStatus } from './types';

export const API_URL = (import.meta.env.VITE_API_URL || 'http://localhost:8084').replace(/\/$/, '');
export const AUTH_URL = (import.meta.env.VITE_AUTH_URL || 'http://localhost:8080').replace(/\/$/, '');
export const CALENDAR_URL = (import.meta.env.VITE_CALENDAR_URL || 'http://localhost:8083').replace(/\/$/, '');

// Told when an admin request answers 401, so the session is re-checked and
// the page can say it expired instead of showing the raw refusal.
let onUnauthorized: () => void = () => undefined;
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler;
}

async function failure(response: Response): Promise<Error> {
  let message = '';
  try {
    const body = (await response.json()) as { error?: string };
    if (body.error) message = body.error;
  } catch {
    // Fall back to the status alone when the response is not JSON.
  }
  return new Error(friendlyError(response.status, message));
}

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
    if (authenticated && response.status === 401) onUnauthorized();
    throw await failure(response);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const sprinterApi = {
  status: () => request<SprinterStatus>('/v1/auth/status', {}, true),
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
    if (!response.ok) throw await failure(response);
    return response.json() as Promise<Scope[]>;
  },
};

