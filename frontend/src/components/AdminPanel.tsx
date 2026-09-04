import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { localDate, localInput } from '../date';
import type { EventSeries, Scope } from '../types';

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

function stateStyle(state: EventSeries['state']) {
  if (state === 'PUBLISHED') return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-400/15 dark:text-emerald-200';
  if (state === 'CANCELLED') return 'bg-red-100 text-red-800 dark:bg-red-400/15 dark:text-red-200';
  return 'bg-amber-100 text-amber-800 dark:bg-amber-400/15 dark:text-amber-200';
}

export function AdminPanel(props: AdminPanelProps) {
  const [tab, setTab] = useState<'events' | 'scopes'>('events');
  const [scopeKind, setScopeKind] = useState<'PROJECT' | 'SUBTEAM'>('PROJECT');
  const [scopeName, setScopeName] = useState('');
  const team = props.scopes.find((scope) => scope.kind === 'TEAM');
  const projects = props.scopes.filter((scope) => scope.kind === 'PROJECT');
  const [parentId, setParentId] = useState(team?.id || '');
  const [savingScope, setSavingScope] = useState(false);
  const [scopeError, setScopeError] = useState('');
  const sortedEvents = useMemo(() => [...props.events].sort((a, b) => a.starts_at_local.localeCompare(b.starts_at_local)), [props.events]);

  useEffect(() => {
    if (!parentId && team) setParentId(team.id);
  }, [parentId, team]);

  async function addScope(event: FormEvent) {
    event.preventDefault();
    setSavingScope(true);
    setScopeError('');
    try {
      await props.onCreateScope(scopeKind, scopeName, parentId);
      setScopeName('');
    } catch (caught) {
      setScopeError(caught instanceof Error ? caught.message : 'Could not create the calendar.');
    } finally {
      setSavingScope(false);
    }
  }

  function changeKind(kind: 'PROJECT' | 'SUBTEAM') {
    setScopeKind(kind);
    setParentId(kind === 'PROJECT' ? team?.id || '' : projects.find((project) => project.status === 'ACTIVE')?.id || '');
  }

  return (
    <main className="mx-auto w-full max-w-[1200px] px-4 pb-16 pt-8 sm:px-6 lg:px-8">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div><p className="mb-2 text-xs font-bold uppercase tracking-[0.2em] text-brand dark:text-soft">Calendar editor</p><h1 className="text-3xl font-semibold tracking-[-0.04em] sm:text-4xl">Keep the schedule useful.</h1></div>
        {tab === 'events' && <button type="button" onClick={props.onCreateEvent} className="min-h-12 rounded-full bg-deep px-6 text-sm font-semibold text-white hover:bg-brand">Create event</button>}
      </div>
      <div className="mb-6 flex gap-1 rounded-full bg-ink/5 p-1 dark:bg-white/10 sm:w-fit">
        <button type="button" onClick={() => setTab('events')} className={`min-h-10 flex-1 rounded-full px-5 text-sm font-semibold sm:flex-none ${tab === 'events' ? 'bg-white text-ink dark:bg-soft dark:text-ink' : 'text-ink/60 dark:text-white/60'}`}>Events</button>
        <button type="button" onClick={() => setTab('scopes')} className={`min-h-10 flex-1 rounded-full px-5 text-sm font-semibold sm:flex-none ${tab === 'scopes' ? 'bg-white text-ink dark:bg-soft dark:text-ink' : 'text-ink/60 dark:text-white/60'}`}>Projects & subteams</button>
      </div>
      {props.error && <p className="mb-5 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{props.error}</p>}
      {props.loading && <div className="mb-4 h-1 animate-pulse rounded-full bg-brand" />}

      {tab === 'events' ? (
        <section className="overflow-hidden rounded-3xl border border-ink/10 bg-white dark:border-white/10 dark:bg-white/[0.04]">
          {sortedEvents.length === 0 ? <div className="px-6 py-16 text-center"><p className="font-semibold">No events yet.</p><p className="mt-2 text-sm text-ink/60 dark:text-white/60">Create the first team, project, or subteam event.</p></div> : (
            <div className="divide-y divide-ink/10 dark:divide-white/10">
              {sortedEvents.map((event) => (
                <article key={event.id} className="grid gap-4 p-5 sm:grid-cols-[1fr_auto] sm:items-center">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2"><h2 className="truncate font-semibold">{event.title}</h2><span className={`rounded-full px-2.5 py-1 text-[11px] font-bold ${stateStyle(event.state)}`}>{event.state.toLowerCase()}</span></div>
                    <p className="mt-1 text-sm text-ink/60 dark:text-white/60">{event.scope_name} · {localDate(event.starts_at_local)} at {localInput(event.starts_at_local).slice(11)}{event.recurrence_until ? ` · weekly through ${localDate(event.recurrence_until)}` : ''}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <button type="button" onClick={() => props.onEditEvent(event)} className="min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-white/20 dark:hover:bg-white/10">Edit</button>
                    {event.state !== 'PUBLISHED' && <button type="button" onClick={() => void props.onPublishEvent(event)} className="min-h-11 rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand">Publish</button>}
                    {event.state === 'PUBLISHED' && <button type="button" onClick={() => { if (window.confirm('Cancel this entire series? Subscribers will receive the cancellation.')) void props.onCancelEvent(event); }} className="min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30">Cancel series</button>}
                    {event.state === 'DRAFT' && <button type="button" onClick={() => { if (window.confirm('Permanently delete this draft?')) void props.onDeleteEvent(event); }} className="min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30">Delete</button>}
                  </div>
                </article>
              ))}
            </div>
          )}
        </section>
      ) : (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <section className="overflow-hidden rounded-3xl border border-ink/10 bg-white dark:border-white/10 dark:bg-white/[0.04]">
            <div className="border-b border-ink/10 px-5 py-4 dark:border-white/10"><h2 className="font-semibold">Calendar structure</h2><p className="mt-1 text-sm text-ink/60 dark:text-white/60">Archived calendars keep their history and subscription URL.</p></div>
            <div className="divide-y divide-ink/10 dark:divide-white/10">
              {projects.map((project) => (
                <ScopeGroup key={project.id} project={project} subteams={props.scopes.filter((scope) => scope.parent_id === project.id)} {...props} />
              ))}
            </div>
          </section>
          <aside className="h-fit rounded-3xl border border-ink/10 bg-white p-5 dark:border-white/10 dark:bg-white/[0.04]">
            <h2 className="font-semibold">Add a calendar</h2>
            <form onSubmit={(event) => void addScope(event)} className="mt-4 space-y-4">
              <label className="block"><span className="text-sm font-semibold">Type</span><select value={scopeKind} onChange={(e) => changeKind(e.target.value as 'PROJECT' | 'SUBTEAM')} className="mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 text-sm dark:border-white/20"><option value="PROJECT">Project</option><option value="SUBTEAM">Subteam</option></select></label>
              <label className="block"><span className="text-sm font-semibold">Name</span><input required minLength={2} maxLength={80} value={scopeName} onChange={(e) => setScopeName(e.target.value)} className="mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 text-sm dark:border-white/20" /></label>
              {scopeKind === 'SUBTEAM' && <label className="block"><span className="text-sm font-semibold">Project</span><select required value={parentId} onChange={(e) => setParentId(e.target.value)} className="mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 text-sm dark:border-white/20">{projects.filter((project) => project.status === 'ACTIVE').map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>}
              {scopeError && <p className="text-sm text-red-700 dark:text-red-300">{scopeError}</p>}
              <button type="submit" disabled={savingScope || !parentId} className="min-h-11 w-full rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand disabled:opacity-50">Add {scopeKind === 'PROJECT' ? 'project' : 'subteam'}</button>
            </form>
            <p className="mt-5 text-xs leading-5 text-ink/50 dark:text-white/50">Permanent deletion is available only for empty calendars. Archive anything with event history.</p>
          </aside>
        </div>
      )}
    </main>
  );
}

function ScopeGroup({ project, subteams, onRenameScope, onArchiveScope, onRestoreScope, onDeleteScope }: {
  project: Scope;
  subteams: Scope[];
  onRenameScope: AdminPanelProps['onRenameScope'];
  onArchiveScope: AdminPanelProps['onArchiveScope'];
  onRestoreScope: AdminPanelProps['onRestoreScope'];
  onDeleteScope: AdminPanelProps['onDeleteScope'];
}) {
  const actionRow = (scope: Scope, nested = false) => (
    <div className={`flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center ${nested ? 'bg-ink/[0.018] pl-9 dark:bg-black/10' : ''}`}>
      <div className="min-w-0 flex-1"><div className="flex items-center gap-2"><p className="truncate font-semibold">{scope.name}</p>{scope.status === 'ARCHIVED' && <span className="rounded-full bg-ink/8 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider dark:bg-white/10">Archived</span>}</div><p className="mt-1 text-xs text-ink/50 dark:text-white/50">{scope.event_count} events{scope.kind === 'PROJECT' ? ` · ${scope.child_count} subteams` : ''}</p></div>
      <div className="flex flex-wrap gap-1">
        <button type="button" onClick={() => { const name = window.prompt('Calendar name', scope.name); if (name && name !== scope.name) void onRenameScope(scope, name); }} className="min-h-10 rounded-full px-3 text-xs font-semibold hover:bg-soft/25 dark:hover:bg-white/10">Rename</button>
        {scope.status === 'ACTIVE' ? <button type="button" onClick={() => { if (window.confirm(`Archive ${scope.name}? Existing subscriptions will keep working.`)) void onArchiveScope(scope); }} className="min-h-10 rounded-full px-3 text-xs font-semibold hover:bg-soft/25 dark:hover:bg-white/10">Archive</button> : <button type="button" onClick={() => void onRestoreScope(scope)} className="min-h-10 rounded-full px-3 text-xs font-semibold hover:bg-soft/25 dark:hover:bg-white/10">Restore</button>}
        <button type="button" onClick={() => { if (window.confirm(`Permanently delete ${scope.name}? This succeeds only if it has no events or children.`)) void onDeleteScope(scope); }} className="min-h-10 rounded-full px-3 text-xs font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30">Delete</button>
      </div>
    </div>
  );
  return <div>{actionRow(project)}{subteams.map((subteam) => <div key={subteam.id}>{actionRow(subteam, true)}</div>)}</div>;
}
