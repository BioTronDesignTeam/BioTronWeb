import { useState } from 'react';
import type { Guard, GuardSubject } from '../types';
import { cancelClass, fieldClass, panelClass, primaryClass } from './classes';
import { idsToText, readGuardForm, SUBJECT_LABELS, SUBJECTS, type GuardBody } from './guardForm';

interface GuardsPanelProps {
  guards: Guard[];
  onSave: (subject: GuardSubject, body: GuardBody) => Promise<void>;
  onClear: (subject: GuardSubject) => Promise<void>;
}

export function GuardsPanel({ guards, onSave, onClear }: GuardsPanelProps) {
  return (
    <section className={panelClass}>
      <div className="border-b border-ink/10 px-5 py-4 dark:border-line">
        <h2 className="font-semibold">Guards</h2>
        <p className="mt-1 text-sm text-ink/60 dark:text-muted">
          An empty channel ids list means any channel. A guard needs at least one role; to turn a
          feature off, clear the guard.
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

function GuardRow({
  subject,
  guard,
  onSave,
  onClear,
}: {
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
  // still needs to be reflected once the guard prop actually changes. The
  // fields follow it during render, as React suggests, not in an effect.
  const [shownGuard, setShownGuard] = useState(guard);
  if (guard !== shownGuard) {
    setShownGuard(guard);
    setGuildId(guard?.guild_id ?? '');
    setRoleIds(idsToText(guard?.role_ids ?? []));
    setChannelIds(idsToText(guard?.channel_ids ?? []));
  }

  async function save() {
    setError('');
    const form = readGuardForm(guildId, roleIds, channelIds);
    if ('error' in form) {
      setError(form.error);
      return;
    }
    setSaving(true);
    try {
      await onSave(subject, form.body);
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
        <span className="mb-1 block text-xs font-semibold text-ink/60 sm:hidden dark:text-muted">
          Guild id
        </span>
        <input
          value={guildId}
          onChange={(event) => setGuildId(event.target.value)}
          placeholder="Guild id"
          className={fieldClass}
        />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-semibold text-ink/60 sm:hidden dark:text-muted">
          Role ids
        </span>
        <input
          value={roleIds}
          onChange={(event) => setRoleIds(event.target.value)}
          placeholder="Role ids, comma-separated"
          className={fieldClass}
        />
      </label>
      <label className="block">
        <span className="mb-1 block text-xs font-semibold text-ink/60 sm:hidden dark:text-muted">
          Channel ids
        </span>
        <input
          value={channelIds}
          onChange={(event) => setChannelIds(event.target.value)}
          placeholder="Channel ids, comma-separated"
          className={fieldClass}
        />
      </label>
      <div className="flex gap-2">
        <button
          type="button"
          disabled={saving}
          onClick={() => void save()}
          className={primaryClass}
        >
          Save
        </button>
        <button
          type="button"
          disabled={saving}
          onClick={() => void clear()}
          className={cancelClass}
        >
          Clear
        </button>
      </div>
      {error && <p className="text-sm text-red-700 sm:col-span-5 dark:text-red-300">{error}</p>}
    </div>
  );
}
