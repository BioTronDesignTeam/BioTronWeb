import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { calendarApi } from './api';
import { fullDateTimeLabel, stepAnchor, todayAnchor, viewRange, type CalendarView } from './date';
import type { AuthStatus, EventPayload, EventSeries, Occurrence, Scope } from './types';
import { AdminPanel } from './components/AdminPanel';
import { ConfirmDialog } from './components/ConfirmDialog';
import { EventDetails } from './components/EventDetails';
import { EventEditor, type EventDraft } from './components/EventEditor';
import { Header } from './components/Header';
import { OccurrenceEditor } from './components/OccurrenceEditor';
import { CalendarToolbar, PublicCalendar } from './components/PublicCalendar';
import { Sidebar } from './components/Sidebar';
import { SubscribePanel } from './components/SubscribePanel';
import { ViewSwitch } from './components/ViewSwitch';

export function App() {
  const [auth, setAuth] = useState<AuthStatus>({ can_write: false });
  const [scopes, setScopes] = useState<Scope[]>([]);
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [view, setView] = useState<CalendarView>('month');
  // A day inside the range on screen. The arrows move it by the view's own unit.
  const [anchor, setAnchor] = useState(() => todayAnchor());
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  // Open beside the grid on a wide screen, closed on a narrow one where it would cover the calendar.
  const [sidebarOpen, setSidebarOpen] = useState(() => window.matchMedia('(min-width: 1024px)').matches);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [managing, setManaging] = useState(false);
  const [adminScopes, setAdminScopes] = useState<Scope[]>([]);
  const [adminEvents, setAdminEvents] = useState<EventSeries[]>([]);
  const [adminLoading, setAdminLoading] = useState(false);
  const [adminError, setAdminError] = useState('');
  const [showSubscribe, setShowSubscribe] = useState(
    () => new URLSearchParams(window.location.search).get('subscribe') === '1',
  );
  const [selectedOccurrence, setSelectedOccurrence] = useState<Occurrence>();
  const [editingEvent, setEditingEvent] = useState<EventSeries | null>();
  const [creatingEvent, setCreatingEvent] = useState(false);
  // Set when the new event came from a click on a slot, which fixes where it starts.
  const [eventDraft, setEventDraft] = useState<EventDraft>();
  const [cancellingSeries, setCancellingSeries] = useState<Occurrence>();
  const [editingOccurrence, setEditingOccurrence] = useState<{ occurrence: Occurrence; series: EventSeries }>();
  const [cancellingOccurrence, setCancellingOccurrence] = useState<Occurrence>();
  const [eventActionError, setEventActionError] = useState('');

  // The site links here with ?subscribe=1. Leaving it in the address bar meant
  // closing the panel and reloading, or sharing the link, reopened it forever.
  useEffect(() => {
    const url = new URL(window.location.href);
    if (!url.searchParams.has('subscribe')) return;
    url.searchParams.delete('subscribe');
    window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
  }, []);

  const range = useMemo(() => viewRange(view, anchor), [view, anchor]);

  // No selection means every calendar, which is the resting state of the
  // filter and what its Clear button returns to.
  const visibleOccurrences = useMemo(
    () => (selectedScopes.length === 0
      ? occurrences
      : occurrences.filter((occurrence) => selectedScopes.includes(occurrence.scope_id))),
    [occurrences, selectedScopes],
  );

  const loadAuth = useCallback(async () => {
    try {
      setAuth(await calendarApi.authStatus());
    } catch {
      setAuth({ can_write: false });
    }
  }, []);

  const loadScopes = useCallback(async () => {
    setScopes(await calendarApi.scopes());
  }, []);

  // Month and scope changes overlap on a slow connection, and whichever
  // response landed last used to win: three taps on "Next month" could paint
  // February's events onto April's grid. Every load cancels the one before it
  // and only the newest request is allowed to touch state.
  const occurrenceRequest = useRef(0);
  const occurrenceAbort = useRef<AbortController>(undefined);
  const occurrencesInFlight = useRef(0);

  const loadOccurrences = useCallback(async () => {
    occurrenceAbort.current?.abort();
    const controller = new AbortController();
    occurrenceAbort.current = controller;
    const request = ++occurrenceRequest.current;
    occurrencesInFlight.current += 1;
    setLoading(true);
    setError('');
    try {
      const next = await calendarApi.occurrences(range.from, range.to, undefined, controller.signal);
      if (request === occurrenceRequest.current) setOccurrences(next);
    } catch (caught) {
      if (request !== occurrenceRequest.current || controller.signal.aborted) return;
      // Leaving the previous month's events on screen under an error banner
      // reads as if they belong to the month that failed.
      setOccurrences([]);
      setError(caught instanceof Error ? caught.message : 'Could not load the calendar.');
    } finally {
      occurrencesInFlight.current -= 1;
      // The progress bar belongs to the newest request rather than the first
      // one to settle, so it stays up while a superseded fetch unwinds.
      setLoading(occurrencesInFlight.current > 0);
    }
  }, [range.from, range.to]);

  const loadAdmin = useCallback(async () => {
    if (!auth.can_write) return;
    setAdminLoading(true);
    setAdminError('');
    try {
      const [nextScopes, nextEvents] = await Promise.all([calendarApi.adminScopes(), calendarApi.adminEvents()]);
      setAdminScopes(nextScopes);
      setAdminEvents(nextEvents);
    } catch (caught) {
      setAdminError(caught instanceof Error ? caught.message : 'Could not load calendar administration.');
    } finally {
      setAdminLoading(false);
    }
  }, [auth.can_write]);

  useEffect(() => {
    void Promise.all([loadAuth(), loadScopes()]).catch(() => setError('Could not load the calendar.'));
  }, [loadAuth, loadScopes]);

  useEffect(() => {
    void loadOccurrences();
    return () => occurrenceAbort.current?.abort();
  }, [loadOccurrences]);
  useEffect(() => { if (managing) void loadAdmin(); }, [managing, loadAdmin]);

  // A write can land and the reload behind it still fail. The editor is closed
  // by then, so its own error slot is gone; say so on the page instead of
  // leaving a stale list that looks like the save was ignored.
  async function refreshAfterMutation() {
    try {
      await Promise.all([loadScopes(), loadOccurrences(), loadAdmin()]);
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : 'Could not reload the calendar.';
      setError(`The change was saved, but the calendar could not be reloaded: ${message}`);
      setAdminError(message);
    }
  }

  async function saveEvent(payload: EventPayload, publish: boolean) {
    let saved: EventSeries;
    if (editingEvent) saved = await calendarApi.updateEvent(editingEvent.id, payload);
    else saved = await calendarApi.createEvent(payload);
    if (publish) {
      // Saving and publishing are separate writes. Keep the saved identity and
      // sequence so a failed publish can be retried without creating a duplicate.
      setEditingEvent(saved);
      setCreatingEvent(false);
      await calendarApi.publishEvent(saved.id, saved.sequence);
    }
    setEditingEvent(undefined);
    setCreatingEvent(false);
    setSelectedOccurrence(undefined);
    await refreshAfterMutation();
  }

  // Both editor actions on an occurrence need its series: one to edit it, the
  // other to know what the occurrence would look like without its override.
  // Every failure here used to be swallowed, so the button simply did nothing.
  async function seriesFor(occurrence: Occurrence): Promise<EventSeries> {
    const known = adminEvents.find((candidate) => candidate.id === occurrence.series_id);
    if (known) return known;
    const events = await calendarApi.adminEvents();
    setAdminEvents(events);
    const found = events.find((candidate) => candidate.id === occurrence.series_id);
    if (!found) throw new Error('This event series is no longer available. Reload the calendar and try again.');
    return found;
  }

  async function openEditor(occurrence: Occurrence, target: 'series' | 'occurrence') {
    setEventActionError('');
    try {
      const series = await seriesFor(occurrence);
      if (target === 'series') setEditingEvent(series);
      else setEditingOccurrence({ occurrence, series });
      setSelectedOccurrence(undefined);
    } catch (caught) {
      setEventActionError(caught instanceof Error ? caught.message : 'Could not open the event for editing.');
    }
  }

  async function mutate(action: () => Promise<unknown>, showOnEvent = false) {
    setAdminError('');
    if (showOnEvent) setEventActionError('');
    try {
      await action();
      await refreshAfterMutation();
      return true;
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : 'The calendar change failed.';
      setAdminError(message);
      if (showOnEvent) setEventActionError(message);
      return false;
    }
  }

  const editorScopes = managing ? adminScopes : scopes;
  const showingAdmin = managing && auth.can_write;
  const step = (direction: -1 | 1) => setAnchor((current) => stepAnchor(view, current, direction));
  const viewSwitch = <ViewSwitch view={view} onChange={setView} />;
  // The Manage screen's button. It must not inherit the slot of an earlier click on the grid.
  const startCreating = () => {
    setEventDraft(undefined);
    setCreatingEvent(true);
  };

  return (
    // The calendar view fills the window and never scrolls as a page, so every
    // region inside it must scroll on its own. The Manage view is a long
    // document and scrolls the usual way.
    <div className={`bg-white text-ink dark:bg-page dark:text-white ${showingAdmin ? 'min-h-dvh' : 'flex h-dvh flex-col overflow-hidden'}`}>
      <Header
        auth={auth} managing={managing} onPublic={() => setManaging(false)} onLoggedOut={() => { setAuth({ can_write: false }); setManaging(false); }}
        toolbar={showingAdmin ? undefined : <CalendarToolbar view={view} anchor={anchor} onStep={step} />}
        viewSwitch={showingAdmin ? undefined : viewSwitch}
        sidebar={showingAdmin ? undefined : { open: sidebarOpen, filterCount: selectedScopes.length, onToggle: () => setSidebarOpen((open) => !open) }}
      />
      {showingAdmin ? (
        <AdminPanel
          scopes={adminScopes} events={adminEvents} loading={adminLoading} error={adminError}
          onCreateEvent={startCreating} onEditEvent={(event) => setEditingEvent(event)}
          onPublishEvent={async (event) => { await mutate(() => calendarApi.publishEvent(event.id, event.sequence)); }}
          onCancelEvent={async (event) => { await mutate(() => calendarApi.cancelEvent(event.id, event.sequence)); }}
          onDeleteEvent={async (event) => { await mutate(() => calendarApi.deleteEvent(event.id)); }}
          onCreateScope={async (kind, name, parentId) => { await mutate(() => calendarApi.createScope({ kind, name, parent_id: parentId })); }}
          onRenameScope={async (scope, name) => { await mutate(() => calendarApi.renameScope(scope.id, name)); }}
          onArchiveScope={async (scope) => { await mutate(() => calendarApi.archiveScope(scope.id)); }}
          onRestoreScope={async (scope) => { await mutate(() => calendarApi.restoreScope(scope.id)); }}
          onDeleteScope={async (scope) => { await mutate(() => calendarApi.deleteScope(scope.id)); }}
        />
      ) : (
        <div className="flex min-h-0 flex-1">
          <Sidebar open={sidebarOpen} view={view} anchor={anchor} onStep={step} viewSwitch={viewSwitch} onManage={auth.can_write ? () => setManaging(true) : undefined} scopes={scopes} selectedScopes={selectedScopes} onScopeChange={setSelectedScopes} onClose={() => setSidebarOpen(false)}
            onSubscribe={() => {
              // As a drawer the sidebar would sit open behind the subscribe panel.
              if (!window.matchMedia('(min-width: 1024px)').matches) setSidebarOpen(false);
              setShowSubscribe(true);
            }}
          />
          <PublicCalendar view={view} anchor={anchor} onCreate={auth.can_write ? (draft) => { setEventDraft(draft); setCreatingEvent(true); } : undefined} onOpenDay={(day) => { setAnchor(day); setView('day'); }} occurrences={visibleOccurrences} loading={loading} error={error} onSelectEvent={(occurrence) => { setEventActionError(''); setSelectedOccurrence(occurrence); }} />
        </div>
      )}

      {showSubscribe && <SubscribePanel scopes={scopes} onClose={() => setShowSubscribe(false)} />}
      {selectedOccurrence && <EventDetails occurrence={selectedOccurrence} canWrite={auth.can_write} error={eventActionError} onClose={() => { setEventActionError(''); setSelectedOccurrence(undefined); }} onEditSeries={() => void openEditor(selectedOccurrence, 'series')} onEditOccurrence={() => void openEditor(selectedOccurrence, 'occurrence')} onCancelOccurrence={() => setCancellingOccurrence(selectedOccurrence)} onCancelSeries={() => setCancellingSeries(selectedOccurrence)} />}
      {cancellingOccurrence && (
        <ConfirmDialog
          title="Cancel this occurrence?"
          message={`Only the ${fullDateTimeLabel(cancellingOccurrence)} meeting is cancelled. The rest of the weekly series is unaffected, and subscribers receive the cancellation so their calendars can remove it.`}
          confirmLabel="Cancel this occurrence"
          destructive
          onCancel={() => setCancellingOccurrence(undefined)}
          onConfirm={async () => {
            const occurrence = cancellingOccurrence;
            const success = await mutate(() => calendarApi.updateOccurrence(occurrence.series_id, { recurrence_id_local: occurrence.recurrence_id_local, state: 'CANCELLED', patch: {}, expected_sequence: occurrence.series_sequence }), true);
            setCancellingOccurrence(undefined);
            if (success) setSelectedOccurrence(undefined);
          }}
        />
      )}
      {cancellingSeries && (
        <ConfirmDialog
          title={cancellingSeries.recurring ? 'Cancel this series?' : 'Cancel this event?'}
          message={cancellingSeries.recurring
            ? `Every remaining occurrence of "${cancellingSeries.title}" is cancelled and subscribers receive the cancellation. The series stays in the feed so their calendars can reconcile it, and it can be republished later.`
            : `"${cancellingSeries.title}" is cancelled and subscribers receive the cancellation. It stays in the feed so their calendars can reconcile it, and it can be republished later.`}
          confirmLabel={cancellingSeries.recurring ? 'Cancel the series' : 'Cancel the event'}
          destructive
          onCancel={() => setCancellingSeries(undefined)}
          onConfirm={async () => {
            const occurrence = cancellingSeries;
            const success = await mutate(() => calendarApi.cancelEvent(occurrence.series_id, occurrence.series_sequence), true);
            setCancellingSeries(undefined);
            if (success) setSelectedOccurrence(undefined);
          }}
        />
      )}
      {(creatingEvent || editingEvent) && <EventEditor event={editingEvent || undefined} draft={eventDraft} scopes={editorScopes} onClose={() => { setCreatingEvent(false); setEditingEvent(undefined); }} onSave={saveEvent} />}
      {editingOccurrence && <OccurrenceEditor occurrence={editingOccurrence.occurrence} series={editingOccurrence.series} onClose={() => setEditingOccurrence(undefined)} onSave={async (patch) => {
        const { occurrence } = editingOccurrence;
        await calendarApi.updateOccurrence(occurrence.series_id, { recurrence_id_local: occurrence.recurrence_id_local, state: 'MODIFIED', patch, expected_sequence: occurrence.series_sequence });
        setEditingOccurrence(undefined);
        await refreshAfterMutation();
      }} onReset={async () => {
        const { occurrence } = editingOccurrence;
        await calendarApi.resetOccurrence(occurrence.series_id, occurrence.recurrence_id_local, occurrence.series_sequence);
        setEditingOccurrence(undefined);
        await refreshAfterMutation();
      }} />}
    </div>
  );
}
