import { useState, type FormEvent } from 'react';
import type { Occurrence } from '../types';
import { Modal } from './Modal';

interface OccurrenceEditorProps {
  occurrence: Occurrence;
  onClose: () => void;
  onSave: (patch: Record<string, string | null>) => Promise<void>;
  onReset?: () => Promise<void>;
}

function torontoLocalInput(iso: string) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'America/Toronto', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(new Date(iso));
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}T${value.hour}:${value.minute}`;
}

function addLocalDays(value: string, days: number) {
  const [year, month, day] = value.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day + days, 12)).toISOString().slice(0, 10);
}

const inputClass = 'mt-1 min-h-11 w-full rounded-xl border border-[#16033c]/15 bg-transparent px-3 py-2 text-sm outline-none focus:border-[#3050b0] dark:border-white/20';

export function OccurrenceEditor({ occurrence, onClose, onSave, onReset }: OccurrenceEditorProps) {
  const [title, setTitle] = useState(occurrence.title);
  const [description, setDescription] = useState(occurrence.description);
  const [location, setLocation] = useState(occurrence.location);
  const [url, setURL] = useState(occurrence.url);
  const [startsAt, setStartsAt] = useState(torontoLocalInput(occurrence.starts_at));
  const [endsAt, setEndsAt] = useState(torontoLocalInput(occurrence.ends_at));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  async function submit(formEvent: FormEvent) {
    formEvent.preventDefault();
    setSaving(true);
    setError('');
    try {
      await onSave({
        title,
        description: description || null,
        location: location || null,
        url: url || null,
        starts_at_local: `${startsAt}:00`,
        ends_at_local: `${endsAt}:00`,
      });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not update the occurrence.');
      setSaving(false);
    }
  }

  async function reset() {
    if (!onReset) return;
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
      <p className="mb-5 text-sm leading-6 text-[#16033c]/65 dark:text-white/65">This changes only the selected meeting. The rest of the weekly series keeps its normal schedule.</p>
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
          <div>{onReset && <button type="button" disabled={saving} onClick={() => void reset()} className="min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50 dark:text-red-300 dark:hover:bg-red-950/30">Reset to series</button>}</div>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <button type="button" onClick={onClose} className="min-h-11 rounded-full px-5 text-sm font-semibold hover:bg-[#aedbfc]/25 dark:hover:bg-white/10">Cancel</button>
            <button type="submit" disabled={saving} className="min-h-11 rounded-full bg-[#160b6c] px-5 text-sm font-semibold text-white hover:bg-[#3050b0] disabled:opacity-50">Save occurrence</button>
          </div>
        </div>
      </form>
    </Modal>
  );
}
