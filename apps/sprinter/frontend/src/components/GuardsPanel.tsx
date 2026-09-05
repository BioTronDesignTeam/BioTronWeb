import { useEffect, useState } from 'react';
import type { Guard, GuardSubject } from '../types';

const SUBJECTS: GuardSubject[] = ['agent', 'agent-thread', 'announce', 'nudge'];

const SUBJECT_LABELS: Record<GuardSubject, string> = {
  agent: 'Agent',
  'agent-thread': 'Agent thread',
  announce: 'Announce',
  nudge: 'Nudge',
};

export const panelClass = 'overflow-hidden rounded-3xl border border-ink/10 bg-white dark:border-line dark:bg-surface';
export const fieldClass = 'min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 text-sm dark:border-line-strong dark:bg-surface-deep';
export const actionClass = 'min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-line-strong dark:hover:bg-white/10';
export const destructiveActionClass = 'min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30';
export const primaryClass = 'min-h-11 rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand dark:bg-brand dark:hover:brightness-110 disabled:opacity-50';
export const cancelClass = 'min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-line-strong dark:hover:bg-white/10 disabled:opacity-50';

function idsToText(ids: string[]) {
  return ids.join(', ');
}

function textToIds(text: string) {
  return text.split(',').map((part) => part.trim()).filter(Boolean);
}

function allDigits(ids: string[]) {
  return ids.every((id) => /^\d+$/.test(id));
}

interface GuardsPanelProps {
  guards: Guard[];
  onSave: (subject: GuardSubject, body: { guild_id: string; role_ids: string[]; channel_ids: string[] }) => Promise<void>;
  onClear: (subject: GuardSubject) => Promise<void>;
}

export function GuardsPanel({ guards, onSave, onClear }: GuardsPanelProps) {
  return (
    <section className={panelClass}>
      <div className="border-b border-ink/10 px-5 py-4 dark:border-line">
        <h2 className="font-semibold">Guards</h2>
        <p className="mt-1 text-sm text-ink/60 dark:text-muted">
          An empty channel ids list means any channel. An empty role ids list means nobody.
        </p>
      </div>
      <div className="divide-y divide-ink/10 dark:divide-line">
        {SUBJECTS.map((subject) => (
          <GuardRow
            key={subject}
            subject={subject}
            guard={guards.find((guard) => guard.subject === subject)}
            onSave={onSave}
            onClear={onClear}
          />
        ))}
      </div>
    </section>
  );
}

function GuardRow({ subject, guard, onSave, onClear }: {
  subject: GuardSubject;
  guard?: Guard;
  onSave: GuardsPanelProps['onSave'];
  onClear: GuardsPanelProps['onClear'];
}) {
  const [guildId, setGuildId] = useState(guard?.guild_id ?? '');
  const [roleIds, setRoleIds] = useState(idsToText(guard?.role_ids ?? []));
  const [channelIds, setChannelIds] = useState(idsToText(guard?.channel_ids ?? []));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // The row is uncontrolled from the server's point of view between saves, but
  // a save elsewhere (or the initial load landing after this row mounted)
  // still needs to be reflected once the guard prop actually changes.
  useEffect(() => {
    setGuildId(guard?.guild_id ?? '');
    setRoleIds(idsToText(guard?.role_ids ?? []));
    setChannelIds(idsToText(guard?.channel_ids ?? []));
  }, [guard]);

  async function save() {
    setError('');
    const trimmedGuildId = guildId.trim();
    const roles = textToIds(roleIds);
    const channels = textToIds(channelIds);
    if (!/^\d+$/.test(trimmedGuildId)) {
      setError('Guild id must be a digit string.');
      return;
    }
    if (!allDigits(roles)) {
      setError('Role ids must be digit strings, separated by commas.');
      return;
    }
    if (!allDigits(channels)) {
      setError('Channel ids must be digit strings, separated by commas.');
      return;
    }
    setSaving(true);
    try {
      await onSave(subject, { guild_id: trimmedGuildId, role_ids: roles, channel_ids: channels });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not save the guard.');
    } finally {
      setSaving(false);
    }
  }

  async function clear() {
    setError('');
    setSaving(true);
    try {
      await onClear(subject);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not clear the guard.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="grid gap-3 p-5 sm:grid-cols-[8rem_1fr_1fr_1fr_auto] sm:items-center">
      <p className="font-semibold">{SUBJECT_LABELS[subject]}</p>
      <label className="block">
        <span className="mb-1 block text-xs font-semibold text-ink/60 dark:text-muted sm:hidden">Guild id</span>
        <input value={guildId} onChange={(event) => setGuildId(event.target.value)} placeholder="Guild id" className={fieldClass} />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-semibold text-ink/60 dark:text-muted sm:hidden">Role ids</span>
        <input value={roleIds} onChange={(event) => setRoleIds(event.target.value)} placeholder="Role ids, comma-separated" className={fieldClass} />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-semibold text-ink/60 dark:text-muted sm:hidden">Channel ids</span>
        <input value={channelIds} onChange={(event) => setChannelIds(event.target.value)} placeholder="Channel ids, comma-separated" className={fieldClass} />
      </label>
      <div className="flex gap-2">
        <button type="button" disabled={saving} onClick={() => void save()} className={primaryClass}>Save</button>
        <button type="button" disabled={saving} onClick={() => void clear()} className={cancelClass}>Clear</button>
      </div>
      {error && <p className="text-sm text-red-700 dark:text-red-300 sm:col-span-5">{error}</p>}
    </div>
  );
}
