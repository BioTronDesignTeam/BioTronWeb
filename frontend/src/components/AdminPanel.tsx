import { useMemo, useState, type FormEvent } from 'react';
import { localDate, timeOfDayLabel } from '../date';
import type { EventSeries, Scope } from '../types';
import { ConfirmDialog, PromptDialog } from './ConfirmDialog';

interface AdminPanelProps {
  scopes: Scope[];
  events: EventSeries[];
  loading: boolean;
  error: string;
  onCreateEvent: () => void;
  onEditEvent: (event: EventSeries) => void;
  onPublishEvent: (event: EventSeries) => Promise<void>;
  onCancelEvent: (event: EventSeries) => Promise<void>;
  onDeleteEvent: (event: EventSeries) => Promise<void>;
  onCreateScope: (kind: 'PROJECT' | 'SUBTEAM', name: string, parentId: string) => Promise<void>;
  onRenameScope: (scope: Scope, name: string) => Promise<void>;
  onArchiveScope: (scope: Scope) => Promise<void>;
  onRestoreScope: (scope: Scope) => Promise<void>;
  onDeleteScope: (scope: Scope) => Promise<void>;
}

interface PendingConfirm {
  title: string;
  message: string;
  confirmLabel: string;
  destructive?: boolean;
  run: () => Promise<void>;
}

const SCOPE_NAME_MIN = 2;
const SCOPE_NAME_MAX = 80;

const actionClass = 'min-h-11 rounded-full px-3 text-xs font-semibold hover:bg-soft/25 dark:hover:bg-white/10';
const destructiveActionClass = 'min-h-11 rounded-full px-3 text-xs font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30';
const destructiveEventClass = 'min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30';
const panelClass = 'overflow-hidden rounded-3xl border border-ink/10 bg-white dark:border-line dark:bg-surface';
const fieldClass = 'mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 text-sm dark:border-white/20';

function stateStyle(state: EventSeries['state']) {
  if (state === 'PUBLISHED') return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-400/15 dark:text-emerald-200';
  if (state === 'CANCELLED') return 'bg-red-100 text-red-800 dark:bg-red-400/15 dark:text-red-200';
  return 'bg-amber-100 text-amber-800 dark:bg-amber-400/15 dark:text-amber-200';
}

// Hard deletion succeeds only for an empty leaf scope with no event history,
// and event_count counts drafts and cancelled series too. Offering Delete on
// anything else guaranteed a 409 and a generic conflict banner.
function deleteBlockedReason(scope: Scope) {
  if (scope.kind === 'TEAM') return 'the team root cannot be deleted';
  if (scope.event_count > 0) return 'it still has event history — archive it instead';
  if (scope.child_count > 0) return 'it still has subteams';
  return '';
}

export function AdminPanel(props: AdminPanelProps) {
  const [tab, setTab] = useState<'events' | 'scopes'>('events');
  const [confirming, setConfirming] = useState<PendingConfirm>();
  const [renaming, setRenaming] = useState<Scope>();

  const team = props.scopes.find((scope) => scope.kind === 'TEAM');
  const { projects, activeProjects, subteamsByParent } = useMemo(() => {
    const projects: Scope[] = [];
    const activeProjects: Scope[] = [];
    const subteamsByParent = new Map<string, Scope[]>();
    for (const scope of props.scopes) {
      if (scope.kind === 'PROJECT') {
        projects.push(scope);
        if (scope.status === 'ACTIVE') activeProjects.push(scope);
      } else if (scope.kind === 'SUBTEAM' && scope.parent_id) {
        subteamsByParent.set(scope.parent_id, [...(subteamsByParent.get(scope.parent_id) || []), scope]);
      }
    }
    return { projects, activeProjects, subteamsByParent };
  }, [props.scopes]);

  function confirmThen(pending: Omit<PendingConfirm, 'run'>, run: () => Promise<void>) {
    setConfirming({
      ...pending,
      run: async () => {
        await run();
        setConfirming(undefined);
      },
    });
  }

  const tabClass = (name: 'events' | 'scopes') =>
    `min-h-11 flex-1 rounded-full px-5 text-sm font-semibold sm:flex-none ${tab === name ? 'bg-white text-ink dark:bg-soft dark:text-ink' : 'text-ink/60 dark:text-white/60'}`;

  return (
    <main className="mx-auto w-full max-w-[1200px] px-4 pb-16 pt-8 sm:px-6 lg:px-8">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div><p className="mb-2 text-xs font-bold uppercase tracking-[0.2em] text-brand dark:text-soft">Calendar editor</p><h1 className="text-3xl font-semibold tracking-[-0.04em] sm:text-4xl">Keep the schedule useful.</h1></div>
        {tab === 'events' && <button type="button" onClick={props.onCreateEvent} className="min-h-12 rounded-full bg-deep px-6 text-sm font-semibold text-white hover:bg-brand">Create event</button>}
      </div>
      <div className="mb-6 flex gap-1 rounded-full bg-ink/5 p-1 dark:bg-surface-2 sm:w-fit">
        <button type="button" onClick={() => setTab('events')} className={tabClass('events')}>Events</button>
        <button type="button" onClick={() => setTab('scopes')} className={tabClass('scopes')}>Projects &amp; subteams</button>
      </div>
      {props.error && <p className="mb-5 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{props.error}</p>}
      {props.loading && <div className="mb-4 h-1 animate-pulse rounded-full bg-brand" />}

      {tab === 'events' ? (
        <EventList
          events={props.events}
          onEdit={props.onEditEvent}
          onPublish={props.onPublishEvent}
          onCancel={(event) => confirmThen({
            title: 'Cancel this series?',
            message: `Every remaining occurrence of "${event.title}" is cancelled and subscribers receive the cancellation. The series stays in the feed so their calendars can reconcile it, and it can be republished later.`,
            confirmLabel: 'Cancel the series',
            destructive: true,
          }, () => props.onCancelEvent(event))}
          onDelete={(event) => confirmThen({
            title: 'Delete this draft?',
            message: `"${event.title}" has never been published, so nothing has it yet. Deleting it cannot be undone.`,
            confirmLabel: 'Delete draft',
            destructive: true,
          }, () => props.onDeleteEvent(event))}
        />
      ) : (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <section className={panelClass}>
            <div className="border-b border-ink/10 px-5 py-4 dark:border-white/10"><h2 className="font-semibold">Calendar structure</h2><p className="mt-1 text-sm text-ink/60 dark:text-white/60">Archived calendars keep their history and subscription URL.</p></div>
            <div className="divide-y divide-ink/10 dark:divide-white/10">
              {projects.map((project) => (
                <ScopeGroup
                  key={project.id}
                  project={project}
                  subteams={subteamsByParent.get(project.id) || []}
                  onRename={setRenaming}
                  onArchive={(scope) => confirmThen({
                    title: `Archive ${scope.path}?`,
                    message: 'Its events and its subscription URL keep working exactly as they are. It just stops appearing in the public calendar, and it can be restored later.',
                    confirmLabel: 'Archive',
                  }, () => props.onArchiveScope(scope))}
                  onRestore={props.onRestoreScope}
                  onDelete={(scope) => confirmThen({
                    title: `Permanently delete ${scope.path}?`,
                    message: 'It has no events and no subteams, so nothing is lost, but this cannot be undone.',
                    confirmLabel: 'Delete permanently',
                    destructive: true,
                  }, () => props.onDeleteScope(scope))}
                />
              ))}
            </div>
          </section>
          <AddScopeForm teamId={team?.id || ''} activeProjects={activeProjects} onCreate={props.onCreateScope} />
        </div>
      )}

      {confirming && (
        <ConfirmDialog
          title={confirming.title}
          message={confirming.message}
          confirmLabel={confirming.confirmLabel}
          destructive={confirming.destructive}
          onCancel={() => setConfirming(undefined)}
          onConfirm={confirming.run}
        />
      )}
      {renaming && (
        <PromptDialog
          title="Rename calendar"
          message="Subscription URLs are keyed to the calendar itself, so renaming it never breaks an existing subscription."
          label="Calendar name"
          initialValue={renaming.name}
          confirmLabel="Rename"
          minLength={SCOPE_NAME_MIN}
          maxLength={SCOPE_NAME_MAX}
          onCancel={() => setRenaming(undefined)}
          onConfirm={async (name) => {
            await props.onRenameScope(renaming, name);
            setRenaming(undefined);
          }}
        />
      )}
    </main>
  );
}

function EventList({ events, onEdit, onPublish, onCancel, onDelete }: {
  events: EventSeries[];
  onEdit: (event: EventSeries) => void;
  onPublish: (event: EventSeries) => Promise<void>;
  onCancel: (event: EventSeries) => void;
  onDelete: (event: EventSeries) => void;
}) {
  const sorted = useMemo(
    () => [...events].sort((a, b) => a.starts_at_local.localeCompare(b.starts_at_local)),
    [events],
  );
  if (sorted.length === 0) {
    return (
      <section className={panelClass}>
        <div className="px-6 py-16 text-center">
          <p className="font-semibold">No events yet.</p>
          <p className="mt-2 text-sm text-ink/60 dark:text-white/60">Create the first team, project, or subteam event.</p>
        </div>
      </section>
    );
  }
  return (
    <section className={panelClass}>
      <div className="divide-y divide-ink/10 dark:divide-white/10">
        {sorted.map((event) => (
          <article key={event.id} className="grid gap-4 p-5 sm:grid-cols-[1fr_auto] sm:items-center">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="truncate font-semibold">{event.title}</h2>
                <span className={`rounded-full px-2.5 py-1 text-[11px] font-bold ${stateStyle(event.state)}`}>{event.state.toLowerCase()}</span>
              </div>
              <p className="mt-1 text-sm text-ink/60 dark:text-white/60">
                {event.scope_path} · {localDate(event.starts_at_local)} at {timeOfDayLabel(event.starts_at_local)}
                {event.recurrence_until ? ` · weekly through ${localDate(event.recurrence_until)}` : ''}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button type="button" onClick={() => onEdit(event)} className="min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-white/20 dark:hover:bg-white/10">Edit</button>
              {event.state !== 'PUBLISHED' && <button type="button" onClick={() => void onPublish(event)} className="min-h-11 rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand">{event.state === 'CANCELLED' ? 'Republish' : 'Publish'}</button>}
              {event.state === 'PUBLISHED' && <button type="button" onClick={() => onCancel(event)} className={destructiveEventClass}>Cancel series</button>}
              {event.state === 'DRAFT' && <button type="button" onClick={() => onDelete(event)} className={destructiveEventClass}>Delete</button>}
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function AddScopeForm({ teamId, activeProjects, onCreate }: {
  teamId: string;
  activeProjects: Scope[];
  onCreate: AdminPanelProps['onCreateScope'];
}) {
  const [kind, setKind] = useState<'PROJECT' | 'SUBTEAM'>('PROJECT');
  const [name, setName] = useState('');
  const [chosenParentId, setChosenParentId] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Derived during render rather than corrected by an effect, so the form never
  // paints one frame with an empty parent before catching up.
  const parentId = chosenParentId || (kind === 'PROJECT' ? teamId : activeProjects[0]?.id || '');

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError('');
    try {
      await onCreate(kind, name, parentId);
      setName('');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not create the calendar.');
    } finally {
      setSaving(false);
    }
  }

  return (
    <aside className="h-fit rounded-3xl border border-ink/10 bg-white p-5 dark:border-line dark:bg-surface">
      <h2 className="font-semibold">Add a calendar</h2>
      <form onSubmit={(event) => void submit(event)} className="mt-4 space-y-4">
        <label className="block">
          <span className="text-sm font-semibold">Type</span>
          <select value={kind} onChange={(e) => { setKind(e.target.value as 'PROJECT' | 'SUBTEAM'); setChosenParentId(''); }} className={fieldClass}>
            <option value="PROJECT">Project</option>
            <option value="SUBTEAM">Subteam</option>
          </select>
        </label>
        <label className="block">
          <span className="text-sm font-semibold">Name</span>
          <input required minLength={SCOPE_NAME_MIN} maxLength={SCOPE_NAME_MAX} value={name} onChange={(e) => setName(e.target.value)} className={fieldClass} />
        </label>
        {kind === 'SUBTEAM' && (
          <label className="block">
            <span className="text-sm font-semibold">Project</span>
            <select required value={parentId} onChange={(e) => setChosenParentId(e.target.value)} className={fieldClass}>
              {activeProjects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
            </select>
          </label>
        )}
        {kind === 'SUBTEAM' && activeProjects.length === 0 && (
          <p className="text-sm text-ink/60 dark:text-white/60">A subteam has to sit under an active project. Add or restore a project first.</p>
        )}
        {error && <p className="text-sm text-red-700 dark:text-red-300">{error}</p>}
        <button type="submit" disabled={saving || !parentId} className="min-h-11 w-full rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand disabled:opacity-50">
          Add {kind === 'PROJECT' ? 'project' : 'subteam'}
        </button>
      </form>
      <p className="mt-5 text-xs leading-5 text-ink/50 dark:text-white/50">Permanent deletion is available only for empty calendars. Archive anything with event history.</p>
    </aside>
  );
}

function ScopeGroup({ project, subteams, onRename, onArchive, onRestore, onDelete }: {
  project: Scope;
  subteams: Scope[];
  onRename: (scope: Scope) => void;
  onArchive: (scope: Scope) => void;
  onRestore: (scope: Scope) => Promise<void>;
  onDelete: (scope: Scope) => void;
}) {
  return (
    <div>
      <ScopeRow scope={project} onRename={onRename} onArchive={onArchive} onRestore={onRestore} onDelete={onDelete} />
      {subteams.map((subteam) => (
        <ScopeRow
          key={subteam.id}
          scope={subteam}
          nested
          // Restore refuses while an ancestor is archived, so do not offer it there.
          restorable={project.status === 'ACTIVE'}
          onRename={onRename}
          onArchive={onArchive}
          onRestore={onRestore}
          onDelete={onDelete}
        />
      ))}
    </div>
  );
}

function ScopeRow({ scope, nested = false, restorable = true, onRename, onArchive, onRestore, onDelete }: {
  scope: Scope;
  nested?: boolean;
  restorable?: boolean;
  onRename: (scope: Scope) => void;
  onArchive: (scope: Scope) => void;
  onRestore: (scope: Scope) => Promise<void>;
  onDelete: (scope: Scope) => void;
}) {
  const blocked = deleteBlockedReason(scope);
  return (
    <div className={`flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center ${nested ? 'bg-ink/[0.018] pl-9 dark:bg-surface-2' : ''}`}>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate font-semibold">{scope.name}</p>
          {scope.status === 'ARCHIVED' && <span className="rounded-full bg-ink/8 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider dark:bg-white/10">Archived</span>}
        </div>
        <p className="mt-1 text-xs text-ink/50 dark:text-white/50">
          {scope.event_count} events{scope.kind === 'PROJECT' ? ` · ${scope.child_count} subteams` : ''}
          {blocked ? ` · cannot be deleted: ${blocked}` : ''}
        </p>
      </div>
      <div className="flex flex-wrap gap-1">
        <button type="button" onClick={() => onRename(scope)} className={actionClass}>Rename</button>
        {scope.status === 'ACTIVE' && <button type="button" onClick={() => onArchive(scope)} className={actionClass}>Archive</button>}
        {scope.status === 'ARCHIVED' && restorable && <button type="button" onClick={() => void onRestore(scope)} className={actionClass}>Restore</button>}
        {!blocked && <button type="button" onClick={() => onDelete(scope)} className={destructiveActionClass}>Delete</button>}
      </div>
    </div>
  );
}
