import { useMemo, useState, type FormEvent } from 'react';
import type { EventSeries, Occurrence } from '../types';
import { Modal } from './Modal';

interface OccurrenceEditorProps {
  occurrence: Occurrence;
  series: EventSeries;
  onClose: () => void;
  onSave: (patch: Record<string, string | null>) => Promise<void>;
  onReset: () => Promise<void>;
}

const torontoParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'America/Toronto', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
});

function torontoLocalInput(iso: string) {
  const parts = torontoParts.formatToParts(new Date(iso));
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}T${value.hour}:${value.minute}`;
}

function addLocalDays(value: string, days: number) {
  const [year, month, day] = value.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day + days, 12)).toISOString().slice(0, 10);
}

// Wall-clock arithmetic, done in a UTC frame so no daylight-saving change can
// shift the result. The series stores wall clock and so does the patch.
function wallClock(local: string) {
  return new Date(`${local.slice(0, 19)}Z`);
}

function addWallClockMs(local: string, milliseconds: number) {
  return new Date(wallClock(local).getTime() + milliseconds).toISOString().slice(0, 19);
}

// What this occurrence would be with no override at all: the series values, at
// the generated start its recurrence id names. Anything equal to this is not a
// change and must stay out of the patch, or the occurrence silently stops
// following later series-wide edits.
function seriesBaseline(occurrence: Occurrence, series: EventSeries) {
  const duration = wallClock(series.ends_at_local).getTime() - wallClock(series.starts_at_local).getTime();
  const start = `${occurrence.recurrence_id_local.slice(0, 19)}`;
  return {
    title: series.title,
    description: series.description,
    location: series.location,
    url: series.url,
    starts_at_local: start,
    ends_at_local: addWallClockMs(start, duration),
  };
}

type PatchFields = ReturnType<typeof seriesBaseline>;

const inputClass = 'mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 py-2 text-sm outline-none focus:border-brand dark:border-line-strong dark:bg-surface-deep dark:focus:border-link';

export function OccurrenceEditor({ occurrence, series, onClose, onSave, onReset }: OccurrenceEditorProps) {
  const [title, setTitle] = useState(occurrence.title);
  const [description, setDescription] = useState(occurrence.description);
  const [location, setLocation] = useState(occurrence.location);
  const [url, setURL] = useState(occurrence.url);
  const [startsAt, setStartsAt] = useState(() => torontoLocalInput(occurrence.starts_at));
  const [endsAt, setEndsAt] = useState(() => torontoLocalInput(occurrence.ends_at));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const baseline = useMemo(() => seriesBaseline(occurrence, series), [occurrence, series]);

  function changedFields(): Record<string, string> {
    const edited: PatchFields = {
      title,
      description,
      location,
      url,
      starts_at_local: `${startsAt}:00`,
      ends_at_local: `${endsAt}:00`,
    };
    const patch: Record<string, string> = {};
    for (const field of Object.keys(edited) as (keyof PatchFields)[]) {
      if (edited[field] !== baseline[field]) patch[field] = edited[field];
    }
    return patch;
  }

  async function submit(formEvent: FormEvent) {
    formEvent.preventDefault();
    const patch = changedFields();
    // Nothing differs from the series any more. Saving a full snapshot here was
    // what pinned every field of an untouched occurrence and made it skip later
    // series renames, so instead release it back to the series, or do nothing
    // at all when it was never detached.
    if (Object.keys(patch).length === 0) {
      if (!occurrence.modified) {
        onClose();
        return;
      }
      await reset();
      return;
    }
    setSaving(true);
    setError('');
    try {
      await onSave(patch);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not update the occurrence.');
      setSaving(false);
    }
  }

  async function reset() {
    setSaving(true);
    setError('');
    try {
      await onReset();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not reset the occurrence.');
      setSaving(false);
    }
  }

  return (
    <Modal title="Edit this occurrence" onClose={onClose} wide>
      <p className="mb-5 text-sm leading-6 text-ink/65 dark:text-muted">
        This changes only the selected meeting. The rest of the weekly series keeps its normal schedule, and any field you leave matching the series keeps following later series-wide edits.
      </p>
      <form onSubmit={(formEvent) => void submit(formEvent)}>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="sm:col-span-2"><span className="text-sm font-semibold">Title</span><input required value={title} onChange={(e) => setTitle(e.target.value)} className={inputClass} /></label>
          <label><span className="text-sm font-semibold">Starts</span><input required type={occurrence.all_day ? 'date' : 'datetime-local'} value={occurrence.all_day ? startsAt.slice(0, 10) : startsAt} onChange={(e) => setStartsAt(occurrence.all_day ? `${e.target.value}T00:00` : e.target.value)} className={inputClass} /></label>
          <label><span className="text-sm font-semibold">{occurrence.all_day ? 'Ends (inclusive)' : 'Ends'}</span><input required type={occurrence.all_day ? 'date' : 'datetime-local'} min={occurrence.all_day ? startsAt.slice(0, 10) : undefined} value={occurrence.all_day ? addLocalDays(endsAt.slice(0, 10), -1) : endsAt} onChange={(e) => setEndsAt(occurrence.all_day ? `${addLocalDays(e.target.value, 1)}T00:00` : e.target.value)} className={inputClass} /></label>
          <label><span className="text-sm font-semibold">Location</span><input value={location} onChange={(e) => setLocation(e.target.value)} className={inputClass} /></label>
          <label><span className="text-sm font-semibold">Event link</span><input type="url" value={url} onChange={(e) => setURL(e.target.value)} className={inputClass} /></label>
          <label className="sm:col-span-2"><span className="text-sm font-semibold">Description</span><textarea rows={4} value={description} onChange={(e) => setDescription(e.target.value)} className={`${inputClass} resize-y`} /></label>
        </div>
        {error && <p className="mt-4 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{error}</p>}
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          <div>{occurrence.modified && <button type="button" disabled={saving} onClick={() => void reset()} className="min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50 dark:text-red-300 dark:hover:bg-red-950/30">Reset to series</button>}</div>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <button type="button" onClick={onClose} className="min-h-11 rounded-full px-5 text-sm font-semibold hover:bg-soft/25 dark:hover:bg-white/10">Cancel</button>
            <button type="submit" disabled={saving} className="min-h-11 rounded-full bg-deep px-5 text-sm font-semibold text-white hover:bg-brand dark:bg-brand dark:hover:brightness-110 disabled:opacity-50">Save occurrence</button>
          </div>
        </div>
      </form>
    </Modal>
  );
}
