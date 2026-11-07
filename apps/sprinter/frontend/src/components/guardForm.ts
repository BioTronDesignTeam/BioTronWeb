import type { GuardSubject } from '../types';

export const SUBJECTS: GuardSubject[] = ['agent', 'agent-thread', 'announce', 'nudge'];

export const SUBJECT_LABELS: Record<GuardSubject, string> = {
  agent: 'Agent',
  'agent-thread': 'Agent thread',
  announce: 'Announce',
  nudge: 'Nudge',
};

export function idsToText(ids: string[]) {
  return ids.join(', ');
}

export function textToIds(text: string) {
  return text
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);
}

function allDigits(ids: string[]) {
  return ids.every((id) => /^\d+$/.test(id));
}

export type GuardBody = { guild_id: string; role_ids: string[]; channel_ids: string[] };

/**
 * Checks a guard row as typed. It returns the body to save, or the sentence
 * that says what to fix. The server refuses the same mistakes, but its answer
 * would arrive as a request error rather than beside the row.
 */
export function readGuardForm(
  guildId: string,
  roleIds: string,
  channelIds: string,
): { body: GuardBody } | { error: string } {
  const trimmedGuildId = guildId.trim();
  const roles = textToIds(roleIds);
  const channels = textToIds(channelIds);
  if (!/^\d+$/.test(trimmedGuildId)) return { error: 'Guild id must be a digit string.' };
  // A guard with no role would let nobody in, which nobody means to save.
  if (roles.length === 0) {
    return { error: 'Add at least one role id. To turn this feature off, clear its guard.' };
  }
  if (!allDigits(roles)) return { error: 'Role ids must be digit strings, separated by commas.' };
  if (!allDigits(channels)) {
    return { error: 'Channel ids must be digit strings, separated by commas.' };
  }
  return { body: { guild_id: trimmedGuildId, role_ids: roles, channel_ids: channels } };
}
