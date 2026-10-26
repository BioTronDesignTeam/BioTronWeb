const API_BASE = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');

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
  /** Every component's observed time combined, so a collapsed row has a figure. */
  uptime_24h: number | null;
  uptime_7d: number | null;
  uptime_90d: number | null;
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

export type ApplicationHistory = {
  id: string;
  name: string;
  buckets: HistoryBucket[];
};

export type StatusHistoryResponse = {
  days: number;
  timezone: string;
  /** One daily series per application, the components' days combined. */
  applications: ApplicationHistory[];
  components: ComponentHistory[];
};

export class APIError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

/**
 * Every route is public. No request carries credentials, so the page renders
 * the same for every visitor.
 */
async function publicAPI<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { credentials: 'omit' });
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

export const getStatus = () => publicAPI<StatusResponse>('/v1/status');
export const getStatusHistory = (days = 90) =>
  publicAPI<StatusHistoryResponse>(`/v1/status/history?days=${days}`);

