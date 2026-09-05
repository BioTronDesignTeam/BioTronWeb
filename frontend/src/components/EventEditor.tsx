import { useMemo, useRef, useState, type FormEvent } from 'react';
import { localDate, localInput } from '../date';
import type { EventPayload, EventSeries, Scope } from '../types';
import { Modal } from './Modal';

interface EventEditorProps {
  event?: EventSeries;
  scopes: Scope[];
  onClose: () => void;
  onSave: (payload: EventPayload, publish: boolean) => Promise<void>;
}

const torontoParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'America/Toronto', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
});

function torontoLocalInput(date: Date) {
  const value = Object.fromEntries(torontoParts.formatToParts(date).map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}T${value.hour}:${value.minute}`;
}

function nextHour() {
  const date = new Date();
  date.setMinutes(0, 0, 0);
  date.setTime(date.getTime() + 60 * 60_000);
  return torontoLocalInput(date);
}

function addWallClockHour(value: string) {
  const [datePart, timePart] = value.split('T');
  const [year, month, day] = datePart.split('-').map(Number);
  const [hour, minute] = timePart.split(':').map(Number);
  const wallClock = new Date(Date.UTC(year, month - 1, day, hour, minute));
  wallClock.setUTCHours(wallClock.getUTCHours() + 1);
  return wallClock.toISOString().slice(0, 16);
}

function addLocalDays(value: string, days: number) {
  const [year, month, day] = value.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day + days, 12)).toISOString().slice(0, 10);
}

const inputClass = 'mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 py-2 text-sm outline-none focus:border-brand dark:border-white/20';
const checkboxLabelClass = 'flex min-h-11 items-center gap-3 rounded-xl border border-ink/10 px-3 dark:border-white/15';

interface Schedule {
  startsAt: string;
  endsAt: string;
  allDay: boolean;
  weekly: boolean;
  recurrenceUntil: string;
}

/**
 * The whole all-day / repeat branch, kept out of the editor body so the form
 * itself stays readable. Every input here is either a date or a datetime
 * depending on allDay, which is most of the editor's control flow.
 */
function ScheduleFields({ schedule, warnDroppingOccurrences, onChange }: {
  schedule: Schedule;
  warnDroppingOccurrences: boolean;
  onChange: (next: Partial<Schedule>) => void;
}) {
  const { startsAt, endsAt, allDay, weekly, recurrenceUntil } = schedule;
  return (
    <>
      <label>
        <span className="text-sm font-semibold">Starts</span>
        <input required type={allDay ? 'date' : 'datetime-local'} value={allDay ? startsAt.slice(0, 10) : startsAt} onChange={(e) => onChange({ startsAt: allDay ? `${e.target.value}T00:00` : e.target.value })} className={inputClass} />
      </label>
      <label>
        <span className="text-sm font-semibold">{allDay ? 'Ends (inclusive)' : 'Ends'}</span>
        <input required type={allDay ? 'date' : 'datetime-local'} min={allDay ? startsAt.slice(0, 10) : undefined} value={allDay ? addLocalDays(endsAt.slice(0, 10), -1) : endsAt} onChange={(e) => onChange({ endsAt: allDay ? `${addLocalDays(e.target.value, 1)}T00:00` : e.target.value })} className={inputClass} />
      </label>
      <label className={checkboxLabelClass}>
        <input type="checkbox" checked={allDay} onChange={(e) => onChange({ allDay: e.target.checked })} className="size-4 accent-brand" />
        <span className="text-sm font-semibold">All-day event</span>
      </label>
      <label className={checkboxLabelClass}>
        <input type="checkbox" checked={weekly} onChange={(e) => onChange({ weekly: e.target.checked })} className="size-4 accent-brand" />
        <span className="text-sm font-semibold">Repeat weekly</span>
      </label>
      {weekly && (
        <label className="sm:col-span-2">
          <span className="text-sm font-semibold">Repeat through</span>
          <input required type="date" min={startsAt.slice(0, 10)} value={recurrenceUntil} onChange={(e) => onChange({ recurrenceUntil: e.target.value })} className={inputClass} />
          <span className="mt-1 block text-xs text-ink/50 dark:text-white/50">The end date is inclusive. Schedule a new series when the next term’s meeting time is known.</span>
        </label>
      )}
      {warnDroppingOccurrences && (
        <p className="rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-900 sm:col-span-2 dark:bg-amber-400/10 dark:text-amber-200">
          Turning off the weekly repeat leaves only the first meeting. Every later occurrence disappears from the calendar and from subscribers’ feeds when you save.
        </p>
      )}
    </>
  );
}

export function EventEditor({ event, scopes, onClose, onSave }: EventEditorProps) {
  const [scopeId, setScopeId] = useState(() => event?.scope_id || scopes.find((scope) => scope.kind === 'TEAM')?.id || scopes[0]?.id || '');
  const [title, setTitle] = useState(event?.title || '');
  const [description, setDescription] = useState(event?.description || '');
  const [location, setLocation] = useState(event?.location || '');
  const [url, setURL] = useState(event?.url || '');
  const [schedule, setSchedule] = useState<Schedule>(() => {
    const startsAt = event ? localInput(event.starts_at_local) : nextHour();
    return {
      startsAt,
      endsAt: event ? localInput(event.ends_at_local) : addWallClockHour(startsAt),
      allDay: event?.all_day || false,
      weekly: Boolean(event?.recurrence_until),
      recurrenceUntil: localDate(event?.recurrence_until),
    };
  });
  // Unchecking "All-day event" used to leave a 24-hour block starting at 00:00,
  // because only the checked branch ever touched the times. Remembering them in
  // a ref keeps this out of the render path: it is never displayed, only read
  // back when the checkbox is turned off again.
  const timedTimes = useRef(event?.all_day ? undefined : { startsAt: schedule.startsAt, endsAt: schedule.endsAt });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const selectableScopes = useMemo(
    () => scopes.filter((scope) => scope.status === 'ACTIVE' || scope.id === event?.scope_id),
    [event?.scope_id, scopes],
  );

  function changeSchedule(next: Partial<Schedule>) {
    if (next.allDay === undefined) {
      setSchedule((current) => ({ ...current, ...next }));
      return;
    }
    // Remembering the timed values happens here, in the handler, rather than
    // inside the updater, which React is free to run more than once.
    if (next.allDay) timedTimes.current = { startsAt: schedule.startsAt, endsAt: schedule.endsAt };
    const times = next.allDay ? toAllDay(schedule) : toTimed(schedule, timedTimes.current);
    setSchedule({ ...schedule, ...next, ...times });
  }

  async function submit(eventSubmit: FormEvent<HTMLFormElement>) {
    eventSubmit.preventDefault();
    const submitter = (eventSubmit.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null;
    const publish = submitter?.value === 'publish';
    setSaving(true);
    setError('');
    try {
      await onSave({
        scope_id: scopeId,
        title,
        description,
        location,
        url,
        starts_at_local: `${schedule.startsAt}:00`,
        ends_at_local: `${schedule.endsAt}:00`,
        all_day: schedule.allDay,
        recurrence_until: schedule.weekly ? schedule.recurrenceUntil : '',
        expected_sequence: event?.sequence || 0,
      }, publish);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not save the event.');
      setSaving(false);
    }
  }

  return (
    <Modal title={event ? 'Edit event series' : 'Create event'} onClose={onClose} wide>
      <form onSubmit={(formEvent) => void submit(formEvent)}>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="sm:col-span-2"><span className="text-sm font-semibold">Title</span><input required maxLength={160} value={title} onChange={(e) => setTitle(e.target.value)} className={inputClass} /></label>
          <label><span className="text-sm font-semibold">Calendar</span><select required value={scopeId} disabled={Boolean(event && event.state !== 'DRAFT')} onChange={(e) => setScopeId(e.target.value)} className={inputClass}>
            {selectableScopes.map((scope) => <option key={scope.id} value={scope.id} disabled={scope.status === 'ARCHIVED'}>{scope.kind === 'TEAM' ? 'Teamwide' : `${scope.path}${scope.status === 'ARCHIVED' ? ' (archived)' : ''}`}</option>)}
          </select></label>
          <label><span className="text-sm font-semibold">Location</span><input maxLength={300} value={location} onChange={(e) => setLocation(e.target.value)} className={inputClass} placeholder="E5 2004 or online" /></label>
          <ScheduleFields
            schedule={schedule}
            warnDroppingOccurrences={Boolean(event?.recurrence_until) && !schedule.weekly}
            onChange={changeSchedule}
          />
          <label className="sm:col-span-2"><span className="text-sm font-semibold">Event link</span><input type="url" value={url} onChange={(e) => setURL(e.target.value)} className={inputClass} placeholder="https://" /></label>
          <label className="sm:col-span-2"><span className="text-sm font-semibold">Description</span><textarea rows={4} maxLength={5000} value={description} onChange={(e) => setDescription(e.target.value)} className={`${inputClass} resize-y`} /></label>
        </div>
        {error && <p className="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{error}</p>}
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <button type="button" onClick={onClose} className="min-h-11 rounded-full px-5 text-sm font-semibold hover:bg-soft/25 dark:hover:bg-white/10">Cancel</button>
          <button type="submit" value="draft" disabled={saving} className="min-h-11 rounded-full border border-deep px-5 text-sm font-semibold text-deep disabled:opacity-50 dark:border-link dark:text-link">{event ? 'Save changes' : 'Save draft'}</button>
          {!event && <button type="submit" value="publish" disabled={saving} className="min-h-11 rounded-full bg-deep px-5 text-sm font-semibold text-white hover:bg-brand dark:bg-brand dark:hover:brightness-110 disabled:opacity-50">Create and publish</button>}
        </div>
      </form>
    </Modal>
  );
}

/** All-day boundaries are midnight to midnight, with an exclusive end date. */
function toAllDay({ startsAt, endsAt }: Schedule) {
  const startDate = startsAt.slice(0, 10);
  const endDate = endsAt.slice(0, 10) <= startDate ? startDate : endsAt.slice(0, 10);
  const exclusive = new Date(`${endDate}T12:00:00Z`);
  if (endDate === startDate) exclusive.setUTCDate(exclusive.getUTCDate() + 1);
  return { startsAt: `${startDate}T00:00`, endsAt: `${exclusive.toISOString().slice(0, 10)}T00:00` };
}

/** Restore the times the event had before it was made all-day, or a sensible hour. */
function toTimed({ startsAt }: Schedule, remembered?: { startsAt: string; endsAt: string }) {
  if (remembered) return remembered;
  const start = `${startsAt.slice(0, 10)}T18:00`;
  return { startsAt: start, endsAt: addWallClockHour(start) };
}
