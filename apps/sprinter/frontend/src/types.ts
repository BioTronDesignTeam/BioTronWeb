export type GuardSubject = 'agent' | 'agent-thread' | 'announce' | 'nudge';

export interface Guard {
  subject: GuardSubject;
  guild_id: string;
  role_ids: string[];
  channel_ids: string[];
  updated_at: string;
}

export type AutomationKind = 'ANNOUNCE' | 'NUDGE';
export type DeliverMode = 'DM' | 'CHANNEL';

export interface Automation {
  id: string;
  kind: AutomationKind;
  name: string;
  scope_id: string;
  channel_id: string;
  lead_user_id: string | null;
  lead_hours: number;
  lookback_hours: number;
  any_author: boolean;
  post_hour: number | null;
  deliver: DeliverMode;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AutomationPayload {
  kind: AutomationKind;
  name: string;
  scope_id: string;
  channel_id: string;
  lead_user_id: string | null;
  lead_hours: number;
  lookback_hours: number;
  any_author: boolean;
  post_hour: number | null;
  deliver: DeliverMode;
  enabled: boolean;
}

export interface Operator {
  github_id: number;
  login: string;
  name: string;
  avatar_url: string;
}

export interface AuthStatus {
  operator?: Operator;
  can_admin: boolean;
}

// Copied from apps/calendar/frontend/src/types.ts — this is the exact shape
// GET /v1/scopes on the Calendar API returns, and the dropdown needs the full
// shape (path, in particular) to label nested scopes usefully.
export type ScopeKind = 'TEAM' | 'PROJECT' | 'SUBTEAM';
export type ScopeStatus = 'ACTIVE' | 'ARCHIVED';

export interface Scope {
  id: string;
  kind: ScopeKind;
  name: string;
  slug: string;
  path: string;
  slug_path: string;
  status: ScopeStatus;
  parent_id?: string;
  archived_at?: string;
  child_count: number;
  event_count: number;
}
