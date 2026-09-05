import { fullDateTimeLabel } from '../date';
import type { Occurrence } from '../types';
import { Modal } from './Modal';

interface EventDetailsProps {
  occurrence: Occurrence;
  canWrite: boolean;
  error?: string;
  onClose: () => void;
  onEditSeries: () => void;
  onEditOccurrence: () => void;
  onCancelOccurrence: () => void;
}

export function EventDetails({ occurrence, canWrite, error, onClose, onEditSeries, onEditOccurrence, onCancelOccurrence }: EventDetailsProps) {
  return (
    <Modal title={occurrence.title} onClose={onClose}>
      <div className="flex flex-wrap gap-2">
        <span className="rounded-full bg-brand/10 px-3 py-1 text-xs font-semibold text-brand dark:bg-soft/15 dark:text-soft">{occurrence.scope_path}</span>
        {occurrence.recurring && <span className="rounded-full bg-deep/10 px-3 py-1 text-xs font-semibold text-deep dark:bg-white/10 dark:text-white">Weekly series</span>}
        {occurrence.modified && <span className="rounded-full bg-amber-100 px-3 py-1 text-xs font-semibold text-amber-800 dark:bg-amber-400/15 dark:text-amber-200">Changed occurrence</span>}
      </div>
      <p className="mt-5 text-base font-semibold">{fullDateTimeLabel(occurrence)}</p>
      {occurrence.location && <p className="mt-2 text-sm text-ink/70 dark:text-white/70">{occurrence.location}</p>}
      {occurrence.description && <p className="mt-5 whitespace-pre-wrap text-sm leading-6 text-ink/70 dark:text-white/70">{occurrence.description}</p>}
      {occurrence.url && <a href={occurrence.url} target="_blank" rel="noreferrer" className="mt-5 inline-flex min-h-11 items-center font-semibold text-brand underline decoration-brand/30 underline-offset-4 dark:text-soft">Open event link ↗</a>}
      {error && <p role="alert" className="mt-5 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{error}</p>}
      {canWrite && (
        <div className="mt-7 border-t border-ink/10 pt-5 dark:border-white/10">
          <p className="mb-3 text-xs font-bold uppercase tracking-[0.16em] text-ink/45 dark:text-white/45">Calendar editor</p>
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={onEditSeries} className="min-h-11 rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand">Edit series</button>
            {occurrence.recurring && <button type="button" onClick={onEditOccurrence} className="min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-white/20 dark:hover:bg-white/10">Edit this occurrence</button>}
            {occurrence.recurring && <button type="button" onClick={onCancelOccurrence} className="min-h-11 rounded-full px-4 text-sm font-semibold text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30">Cancel this occurrence</button>}
          </div>
        </div>
      )}
    </Modal>
  );
}
