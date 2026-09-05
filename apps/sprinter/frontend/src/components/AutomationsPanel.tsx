import { useState, type FormEvent } from 'react';
import type { Automation, AutomationKind, AutomationPayload, DeliverMode, Scope } from '../types';
import { actionClass, cancelClass, destructiveActionClass, fieldClass, panelClass, primaryClass } from './GuardsPanel';

function digits(value: string) {
  return /^\d+$/.test(value.trim());
}

function scopeLabel(scopes: Scope[], scopeId: string) {
  return scopes.find((scope) => scope.id === scopeId)?.path ?? scopeId;
}

interface AutomationsPanelProps {
  automations: Automation[];
  scopes: Scope[];
  onCreate: (body: AutomationPayload) => Promise<void>;
  onUpdate: (id: string, body: Partial<AutomationPayload>) => Promise<void>;
  onDelete: (automation: Automation) => Promise<void>;
}

export function AutomationsPanel({ automations, scopes, onCreate, onUpdate, onDelete }: AutomationsPanelProps) {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Automation | null>(null);
  const [rowError, setRowError] = useState('');

  async function toggle(automation: Automation) {
    setRowError('');
    try {
      await onUpdate(automation.id, { enabled: !automation.enabled });
    } catch (caught) {
      setRowError(caught instanceof Error ? caught.message : 'Could not update the automation.');
    }
  }

  async function remove(automation: Automation) {
    if (!window.confirm(`Delete the automation "${automation.name}"? This cannot be undone.`)) return;
    setRowError('');
    try {
      await onDelete(automation);
    } catch (caught) {
      setRowError(caught instanceof Error ? caught.message : 'Could not delete the automation.');
    }
  }

  return (
    <section className={panelClass}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-ink/10 px-5 py-4 dark:border-line">
        <div>
          <h2 className="font-semibold">Automations</h2>
          <p className="mt-1 text-sm text-ink/60 dark:text-muted">Announce posts a calendar reminder to a channel; nudge DMs or pings a lead.</p>
        </div>
        {!creating && !editing && (
          <button type="button" onClick={() => setCreating(true)} className={primaryClass}>New automation</button>
        )}
      </div>
      {rowError && <p className="px-5 pt-4 text-sm text-red-700 dark:text-red-300">{rowError}</p>}
      {automations.length === 0 ? (
        <div className="px-6 py-16 text-center">
          <p className="font-semibold">No automations yet.</p>
          <p className="mt-2 text-sm text-ink/60 dark:text-muted">Create the first announce or nudge automation.</p>
        </div>
      ) : (
        <div className="divide-y divide-ink/10 dark:divide-line">
          {automations.map((automation) => (
            <div key={automation.id} className="grid gap-3 p-5 sm:grid-cols-[1fr_auto] sm:items-center">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="truncate font-semibold">{automation.name}</h3>
                  <span className="rounded-full bg-ink/8 px-2.5 py-1 text-[11px] font-bold uppercase tracking-wider dark:bg-white/10">{automation.kind.toLowerCase()}</span>
                  <span className={`rounded-full px-2.5 py-1 text-[11px] font-bold ${automation.enabled ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-400/15 dark:text-emerald-200' : 'bg-ink/8 dark:bg-white/10'}`}>
                    {automation.enabled ? 'enabled' : 'disabled'}
                  </span>
                </div>
                <p className="mt-1 text-sm text-ink/60 dark:text-muted">
                  {scopeLabel(scopes, automation.scope_id)} · channel {automation.channel_id}
                  {automation.kind === 'ANNOUNCE'
                    ? ` · ${automation.lead_hours}h lead${automation.post_hour !== null ? ` · posts at ${automation.post_hour}:00` : ''}`
                    : ` · ${automation.deliver.toLowerCase()} · ${automation.lead_hours}h lead · ${automation.lookback_hours}h lookback${automation.any_author ? ' · any author' : ''}`}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={() => { setEditing(automation); setCreating(false); }} className={actionClass}>Edit</button>
                <button type="button" onClick={() => void toggle(automation)} className={actionClass}>{automation.enabled ? 'Disable' : 'Enable'}</button>
                <button type="button" onClick={() => void remove(automation)} className={destructiveActionClass}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}
      {(creating || editing) && (
        <AutomationForm
          automation={editing ?? undefined}
          scopes={scopes}
          onCancel={() => { setCreating(false); setEditing(null); }}
          onSave={async (body) => {
            if (editing) await onUpdate(editing.id, body);
            else await onCreate(body as AutomationPayload);
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}

function AutomationForm({ automation, scopes, onCancel, onSave }: {
  automation?: Automation;
  scopes: Scope[];
  onCancel: () => void;
  onSave: (body: AutomationPayload | Partial<AutomationPayload>) => Promise<void>;
}) {
  const [kind, setKind] = useState<AutomationKind>(automation?.kind ?? 'ANNOUNCE');
  const [name, setName] = useState(automation?.name ?? '');
  const [scopeId, setScopeId] = useState(automation?.scope_id ?? scopes[0]?.id ?? '');
  const [channelId, setChannelId] = useState(automation?.channel_id ?? '');
  const [leadHours, setLeadHours] = useState(automation?.lead_hours ?? 24);
  const [postHour, setPostHour] = useState(automation?.post_hour ?? 9);
  const [leadUserId, setLeadUserId] = useState(automation?.lead_user_id ?? '');
  const [lookbackHours, setLookbackHours] = useState(automation?.lookback_hours ?? 24);
  const [anyAuthor, setAnyAuthor] = useState(automation?.any_author ?? false);
  const [deliver, setDeliver] = useState<DeliverMode>(automation?.deliver ?? 'DM');
  const [enabled, setEnabled] = useState(automation?.enabled ?? true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    if (!name.trim()) {
      setError('Name is required.');
      return;
    }
    if (!scopeId) {
      setError('Choose a calendar scope.');
      return;
    }
    if (!digits(channelId)) {
      setError('Channel id must be a digit string.');
      return;
    }
    if (kind === 'ANNOUNCE' && (postHour < 0 || postHour > 23)) {
      setError('Post hour must be between 0 and 23.');
      return;
    }
    if (kind === 'NUDGE' && leadUserId.trim() && !digits(leadUserId)) {
      setError('Lead user id must be a digit string.');
      return;
    }

    const body: AutomationPayload = {
      kind,
      name: name.trim(),
      scope_id: scopeId,
      channel_id: channelId.trim(),
      lead_user_id: kind === 'NUDGE' ? (leadUserId.trim() || null) : null,
      lead_hours: leadHours,
      lookback_hours: kind === 'NUDGE' ? lookbackHours : 0,
      any_author: kind === 'NUDGE' ? anyAuthor : false,
      post_hour: kind === 'ANNOUNCE' ? postHour : null,
      deliver: kind === 'NUDGE' ? deliver : 'CHANNEL',
      enabled,
    };
    setSaving(true);
    try {
      await onSave(body);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not save the automation.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={(event) => void submit(event)} className="space-y-4 border-t border-ink/10 p-5 dark:border-line">
      <h3 className="font-semibold">{automation ? 'Edit automation' : 'New automation'}</h3>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block">
          <span className="text-sm font-semibold">Kind</span>
          {/* An automation's kind sets which fields exist below it, so it is
              fixed once created rather than repurposing one row's storage for
              the other kind's fields. */}
          <select
            disabled={!!automation}
            value={kind}
            onChange={(event) => setKind(event.target.value as AutomationKind)}
            className={`mt-1 ${fieldClass}`}
          >
            <option value="ANNOUNCE">Announce</option>
            <option value="NUDGE">Nudge</option>
          </select>
        </label>
        <label className="block">
          <span className="text-sm font-semibold">Name</span>
          <input required value={name} onChange={(event) => setName(event.target.value)} className={`mt-1 ${fieldClass}`} />
        </label>
        <label className="block">
          <span className="text-sm font-semibold">Calendar scope</span>
          <select required value={scopeId} onChange={(event) => setScopeId(event.target.value)} className={`mt-1 ${fieldClass}`}>
            {scopes.length === 0 && <option value="">No scopes available</option>}
            {scopes.map((scope) => <option key={scope.id} value={scope.id}>{scope.path}</option>)}
          </select>
        </label>
        <label className="block">
          <span className="text-sm font-semibold">Channel id</span>
          <input required value={channelId} onChange={(event) => setChannelId(event.target.value)} className={`mt-1 ${fieldClass}`} />
        </label>
        <label className="block">
          <span className="text-sm font-semibold">Lead hours</span>
          <input
            required
            type="number"
            min={0}
            value={leadHours}
            onChange={(event) => setLeadHours(Number(event.target.value))}
            className={`mt-1 ${fieldClass}`}
          />
        </label>
        {kind === 'ANNOUNCE' && (
          <label className="block">
            <span className="text-sm font-semibold">Post hour (0-23)</span>
            <input
              required
              type="number"
              min={0}
              max={23}
              value={postHour}
              onChange={(event) => setPostHour(Number(event.target.value))}
              className={`mt-1 ${fieldClass}`}
            />
          </label>
        )}
        {kind === 'NUDGE' && (
          <>
            <label className="block">
              <span className="text-sm font-semibold">Lead user id (optional)</span>
              <input value={leadUserId} onChange={(event) => setLeadUserId(event.target.value)} className={`mt-1 ${fieldClass}`} />
            </label>
            <label className="block">
              <span className="text-sm font-semibold">Lookback hours</span>
              <input
                required
                type="number"
                min={0}
                value={lookbackHours}
                onChange={(event) => setLookbackHours(Number(event.target.value))}
                className={`mt-1 ${fieldClass}`}
              />
            </label>
            <label className="block">
              <span className="text-sm font-semibold">Deliver</span>
              <select value={deliver} onChange={(event) => setDeliver(event.target.value as DeliverMode)} className={`mt-1 ${fieldClass}`}>
                <option value="DM">DM</option>
                <option value="CHANNEL">Channel</option>
              </select>
            </label>
            <label className="flex items-center gap-2 pt-6">
              <input type="checkbox" checked={anyAuthor} onChange={(event) => setAnyAuthor(event.target.checked)} />
              <span className="text-sm font-semibold">Any author</span>
            </label>
          </>
        )}
        <label className="flex items-center gap-2 pt-6">
          <input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="text-sm font-semibold">Enabled</span>
        </label>
      </div>
      {error && <p className="text-sm text-red-700 dark:text-red-300">{error}</p>}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={saving} className={primaryClass}>Save</button>
        <button type="button" disabled={saving} onClick={onCancel} className={cancelClass}>Cancel</button>
      </div>
    </form>
  );
}
