import type { Occurrence } from './types';

/** The colour of an event chip, by the kind of calendar it belongs to. */
export function eventTone(occurrence: Occurrence) {
  if (occurrence.scope_kind === 'TEAM') return 'border-brand bg-brand/10 dark:bg-brand/25';
  if (occurrence.scope_kind === 'PROJECT') return 'border-deep bg-deep/8 dark:border-link dark:bg-highlight/70';
  return 'border-soft bg-soft/40 dark:border-line-strong dark:bg-surface-2';
}
