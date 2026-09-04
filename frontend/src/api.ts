const API_BASE = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');
const AUTH_BASE = (import.meta.env.VITE_IDP_URL || 'http://localhost:8080').replace(/\/$/, '');

export type HealthState = 'healthy' | 'unhealthy' | 'unknown';
export type LogLevel = 'debug' | 'info' | 'warning' | 'error';

export type ComponentStatus = {
  id: string;
  name: string;
  state: HealthState;
  detail?: string;
  checked_at?: string;
};

export type ApplicationStatus = {
  id: string;
  name: string;
  description: string;
  state: HealthState;
  components: ComponentStatus[];
};

export type ApplicationsResponse = {
  applications: ApplicationStatus[];
  generated_at: string;
};

export type LogEntry = {
  id: string;
  service: string;
  level: LogLevel;
  message: string;
  payload?: unknown;
  created_at: string;
};

export type LogPage = {
  logs: LogEntry[];
  next_cursor?: string;
};

export type Identity = {
  login: string;
  name: string;
  avatar_url: string;
  is_superuser: boolean;
  is_manager: boolean;
  is_staff: boolean;
  is_guest: boolean;
};

export class APIError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function api<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { credentials: 'include' });
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // The status code remains authoritative when a proxy returns plain text.
    }
    throw new APIError(response.status, message || `HTTP ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export const loginURL = () =>
  `${AUTH_BASE}/auth/github/login?redirect=${encodeURIComponent(window.location.href)}`;

export const accessManagerURL = () => AUTH_BASE.replace(/\/api$/, '');

export const checkSession = () => api<{ allowed: boolean }>('/v1/session');

export async function getIdentity() {
  const response = await fetch(`${AUTH_BASE}/auth/me`, { credentials: 'include' });
  if (!response.ok) throw new APIError(response.status, 'Could not load account');
  return response.json() as Promise<Identity>;
}

export async function logout() {
  const response = await fetch(`${AUTH_BASE}/auth/logout`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'X-Requested-With': 'XMLHttpRequest' },
  });
  if (!response.ok) throw new APIError(response.status, 'Could not sign out');
}

export const getApplications = () => api<ApplicationsResponse>('/v1/apps');

type LogFilters = {
  levels: LogLevel[];
  search: string;
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
};

function logQuery(filters: LogFilters) {
  const query = new URLSearchParams();
  if (filters.levels.length) query.set('levels', filters.levels.join(','));
  if (filters.search) query.set('q', filters.search);
  if (filters.from) query.set('from', filters.from);
  if (filters.to) query.set('to', filters.to);
  if (filters.cursor) query.set('cursor', filters.cursor);
  query.set('limit', String(filters.limit ?? 100));
  return query.toString();
}

export const getRecentLogs = (application: string, filters: LogFilters) =>
  api<LogPage>(`/v1/apps/${encodeURIComponent(application)}/logs/recent?${logQuery(filters)}`);

export const getHistoricalLogs = (application: string, filters: LogFilters) =>
  api<LogPage>(`/v1/apps/${encodeURIComponent(application)}/logs/history?${logQuery(filters)}`);
