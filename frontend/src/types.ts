export type ScopeKind = 'TEAM' | 'PROJECT' | 'SUBTEAM';
export type ScopeStatus = 'ACTIVE' | 'ARCHIVED';
export type EventState = 'DRAFT' | 'PUBLISHED' | 'CANCELLED';

export interface Scope {
  id: string;
  kind: ScopeKind;
  name: string;
  slug: string;
  /** The scope qualified by its ancestors, such as "Exo · Software". Use it
   *  anywhere scopes appear in a flat list; the tree shows `name`. */
  path: string;
  slug_path: string;
  status: ScopeStatus;
  parent_id?: string;
  archived_at?: string;
  child_count: number;
  event_count: number;
}

export interface Occurrence {
  series_id: string;
  uid: string;
  scope_id: string;
  scope_name: string;
  scope_path: string;
  scope_kind: ScopeKind;
  title: string;
  description: string;
  location: string;
  url: string;
  starts_at: string;
  ends_at: string;
  all_day: boolean;
  timezone: string;
  recurrence_id_local: string;
  recurring: boolean;
  modified: boolean;
  series_sequence: number;
  override_sequence?: number;
}

export interface EventOverride {
  id: string;
  recurrence_id_local: string;
  state: 'MODIFIED' | 'CANCELLED';
  patch: Record<string, string | null>;
  sequence: number;
}

export interface EventSeries {
  id: string;
  uid: string;
  scope_id: string;
  scope_name: string;
  scope_path: string;
  scope_kind: ScopeKind;
  state: EventState;
  title: string;
  description: string;
  location: string;
  url: string;
  starts_at_local: string;
  ends_at_local: string;
  timezone: string;
  all_day: boolean;
  recurrence_until?: string;
  sequence: number;
  overrides?: EventOverride[];
}

export interface Operator {
  github_id: number;
  login: string;
  name: string;
  avatar_url: string;
  is_superuser: boolean;
  is_manager: boolean;
  is_staff: boolean;
}

export interface AuthStatus {
  operator?: Operator;
  can_write: boolean;
}

export interface EventPayload {
  scope_id: string;
  title: string;
  description: string;
  location: string;
  url: string;
  starts_at_local: string;
  ends_at_local: string;
  all_day: boolean;
  recurrence_until: string;
  expected_sequence: number;
}
