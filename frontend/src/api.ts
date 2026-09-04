const API_BASE = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');
const AUTH_BASE = (import.meta.env.VITE_IDP_URL || 'http://localhost:8080').replace(/\/$/, '');

export type HealthState = 'healthy' | 'unhealthy' | 'unknown';
export type LogLevel = 'debug' | 'info' | 'warning' | 'error';

/** Coarse public state. Never carries operational detail. */
export type StatusState = 'operational' | 'degraded' | 'down' | 'unknown';

export type StatusComponent = {
  id: string;
  name: string;
  state: StatusState;
  uptime_24h: number | null;
  uptime_7d: number | null;
  uptime_90d: number | null;
  checked_at: string | null;
};

export type StatusApplication = {
  id: string;
  name: string;
  description: string;
  state: StatusState;
  components: StatusComponent[];
};

export type StatusResponse = {
  overall: {
    state: StatusState;
    uptime_24h: number | null;
    uptime_7d: number | null;
    uptime_90d: number | null;
    updated_at: string;
  };
  applications: StatusApplication[];
};

export type HistoryBucket = {
  date: string;
  /** null means nobody was watching that day, which is not the same as 100. */
  uptime: number | null;
  state: StatusState;
};

export type ComponentHistory = {
  id: string;
  name: string;
  application_id: string;
  buckets: HistoryBucket[];
};

export type StatusHistoryResponse = {
  days: number;
  timezone: string;
  components: ComponentHistory[];
};

export type Session = {
  authenticated: boolean;
  allowed: boolean;
};

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

async function request<T>(path: string, credentials: RequestCredentials): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { credentials });
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

const api = <T,>(path: string) => request<T>(path, 'include');
const publicAPI = <T,>(path: string) => request<T>(path, 'omit');

export const loginURL = () =>
  `${AUTH_BASE}/auth/github/login?redirect=${encodeURIComponent(window.location.href)}`;

export const accessManagerURL = () => AUTH_BASE.replace(/\/api$/, '');

/**
 * Public routes. These carry no credentials at all, so the status page renders
 * identically for a signed-out visitor and a signed-in one.
 */
export const getStatus = () => publicAPI<StatusResponse>('/v1/status');
export const getStatusHistory = (days = 90) =>
  publicAPI<StatusHistoryResponse>(`/v1/status/history?days=${days}`);

/**
 * Always answers 200. `authenticated` without `allowed` means a real session
 * that lacks logger/view, and that state must never be offered a sign-in
 * button: signing in again is exactly what does not help.
 */
export const checkSession = () => api<Session>('/v1/session');

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
