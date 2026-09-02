import { useCallback, useEffect, useMemo, useState } from 'react';
import { calendarApi } from './api';
import { calendarRange, startOfMonth } from './date';
import type { AuthStatus, EventPayload, EventSeries, Occurrence, Scope } from './types';
import { AdminPanel } from './components/AdminPanel';
import { EventDetails } from './components/EventDetails';
import { EventEditor } from './components/EventEditor';
import { Header } from './components/Header';
import { OccurrenceEditor } from './components/OccurrenceEditor';
import { PublicCalendar } from './components/PublicCalendar';
import { SubscribePanel } from './components/SubscribePanel';

export function App() {
  const [auth, setAuth] = useState<AuthStatus>({ can_write: false });
  const [scopes, setScopes] = useState<Scope[]>([]);
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [month, setMonth] = useState(() => startOfMonth());
  const [selectedScope, setSelectedScope] = useState('');
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
  const [editingOccurrence, setEditingOccurrence] = useState<Occurrence>();
  const [eventActionError, setEventActionError] = useState('');

  const range = useMemo(() => calendarRange(month), [month]);

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

  const loadOccurrences = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setOccurrences(await calendarApi.occurrences(range.from, range.to, selectedScope || undefined));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not load the calendar.');
    } finally {
      setLoading(false);
    }
  }, [range.from, range.to, selectedScope]);

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

  useEffect(() => { void loadOccurrences(); }, [loadOccurrences]);
  useEffect(() => { if (managing) void loadAdmin(); }, [managing, loadAdmin]);

  async function refreshAfterMutation() {
    await Promise.all([loadScopes(), loadOccurrences(), loadAdmin()]);
  }

  async function saveEvent(payload: EventPayload, publish: boolean) {
    let saved: EventSeries;
    if (editingEvent) saved = await calendarApi.updateEvent(editingEvent.id, payload);
    else saved = await calendarApi.createEvent(payload);
    if (publish) await calendarApi.publishEvent(saved.id, saved.sequence);
    setEditingEvent(undefined);
    setCreatingEvent(false);
    setSelectedOccurrence(undefined);
    await refreshAfterMutation();
  }

  async function editSeriesFromOccurrence() {
    if (!selectedOccurrence) return;
    let event = adminEvents.find((candidate) => candidate.id === selectedOccurrence.series_id);
    if (!event) {
      const events = await calendarApi.adminEvents();
      setAdminEvents(events);
      event = events.find((candidate) => candidate.id === selectedOccurrence.series_id);
    }
    if (event) {
      setEditingEvent(event);
      setSelectedOccurrence(undefined);
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

  return (
    <div className="min-h-dvh bg-[#f8f9ff] text-[#16033c] dark:bg-[#16033c] dark:text-white">
      <Header auth={auth} managing={managing} onManage={() => setManaging(true)} onPublic={() => setManaging(false)} onLoggedOut={() => { setAuth({ can_write: false }); setManaging(false); }} />
      {managing && auth.can_write ? (
        <AdminPanel
          scopes={adminScopes} events={adminEvents} loading={adminLoading} error={adminError}
          onCreateEvent={() => setCreatingEvent(true)} onEditEvent={(event) => setEditingEvent(event)}
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
        <PublicCalendar month={month} scopes={scopes} occurrences={occurrences} selectedScope={selectedScope} loading={loading} error={error} onMonthChange={setMonth} onScopeChange={setSelectedScope} onSubscribe={() => setShowSubscribe(true)} onSelectEvent={(occurrence) => { setEventActionError(''); setSelectedOccurrence(occurrence); }} />
      )}
      <footer className="border-t border-[#16033c]/10 px-4 py-6 text-center text-xs text-[#16033c]/50 dark:border-white/10 dark:text-white/45">Times use America/Toronto · Calendar subscriptions update on each calendar app’s schedule</footer>

      {showSubscribe && <SubscribePanel scopes={scopes} onClose={() => setShowSubscribe(false)} />}
      {selectedOccurrence && <EventDetails occurrence={selectedOccurrence} canWrite={auth.can_write} error={eventActionError} onClose={() => { setEventActionError(''); setSelectedOccurrence(undefined); }} onEditSeries={() => void editSeriesFromOccurrence()} onEditOccurrence={() => { setEditingOccurrence(selectedOccurrence); setSelectedOccurrence(undefined); }} onCancelOccurrence={() => {
        const occurrence = selectedOccurrence;
        if (window.confirm('Cancel only this occurrence? Subscribers will receive the cancellation.')) {
          void mutate(() => calendarApi.updateOccurrence(occurrence.series_id, { recurrence_id_local: occurrence.recurrence_id_local, state: 'CANCELLED', patch: {}, expected_sequence: occurrence.series_sequence }), true).then((success) => {
            if (success) setSelectedOccurrence(undefined);
          });
        }
      }} />}
      {(creatingEvent || editingEvent) && <EventEditor event={editingEvent || undefined} scopes={editorScopes} onClose={() => { setCreatingEvent(false); setEditingEvent(undefined); }} onSave={saveEvent} />}
      {editingOccurrence && <OccurrenceEditor occurrence={editingOccurrence} onClose={() => setEditingOccurrence(undefined)} onSave={async (patch) => {
        await calendarApi.updateOccurrence(editingOccurrence.series_id, { recurrence_id_local: editingOccurrence.recurrence_id_local, state: 'MODIFIED', patch, expected_sequence: editingOccurrence.series_sequence });
        setEditingOccurrence(undefined);
        await refreshAfterMutation();
      }} onReset={editingOccurrence.modified ? async () => {
        await calendarApi.resetOccurrence(editingOccurrence.series_id, editingOccurrence.recurrence_id_local, editingOccurrence.series_sequence);
        setEditingOccurrence(undefined);
        await refreshAfterMutation();
      } : undefined} />}
    </div>
  );
}
