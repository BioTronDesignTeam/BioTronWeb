import { useState, type FormEvent } from 'react';
import { Modal } from './Modal';

const cancelClass = 'min-h-11 rounded-full px-5 text-sm font-semibold hover:bg-soft/25 dark:hover:bg-white/10';
const primaryClass = 'min-h-11 rounded-full bg-deep px-5 text-sm font-semibold text-white hover:bg-brand disabled:opacity-50';
const destructiveClass = 'min-h-11 rounded-full bg-red-700 px-5 text-sm font-semibold text-white hover:bg-red-800 disabled:opacity-50';

interface ConfirmDialogProps {
  title: string;
  message: string;
  confirmLabel: string;
  destructive?: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
}

/**
 * The app already ships a focus-trapping, Escape-handling, scroll-locking
 * Modal, so a destructive action should not drop the user into unstyled
 * browser chrome that no theme, no keyboard trap, and no wording of ours
 * reaches.
 */
export function ConfirmDialog({ title, message, confirmLabel, destructive = false, onCancel, onConfirm }: ConfirmDialogProps) {
  const [busy, setBusy] = useState(false);

  async function confirm() {
    setBusy(true);
    try {
      await onConfirm();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={title} onClose={onCancel}>
      <p className="text-sm leading-6 text-ink/70 dark:text-white/70">{message}</p>
      <div className="mt-7 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <button type="button" onClick={onCancel} className={cancelClass}>Keep as is</button>
        <button type="button" disabled={busy} onClick={() => void confirm()} className={destructive ? destructiveClass : primaryClass}>{confirmLabel}</button>
      </div>
    </Modal>
  );
}

interface PromptDialogProps {
  title: string;
  message: string;
  label: string;
  initialValue: string;
  confirmLabel: string;
  minLength: number;
  maxLength: number;
  onCancel: () => void;
  onConfirm: (value: string) => void | Promise<void>;
}

/**
 * window.prompt had no validation at all, so a name the server rejects was
 * only discovered after the round trip, as a red banner somewhere else on the
 * page. The same limits the API enforces are applied here, before sending.
 */
export function PromptDialog({ title, message, label, initialValue, confirmLabel, minLength, maxLength, onCancel, onConfirm }: PromptDialogProps) {
  const [value, setValue] = useState(initialValue);
  const [busy, setBusy] = useState(false);
  const trimmed = value.trim();
  const invalid = trimmed.length < minLength || trimmed.length > maxLength;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (invalid || trimmed === initialValue) return;
    setBusy(true);
    try {
      await onConfirm(trimmed);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={title} onClose={onCancel}>
      <form onSubmit={(event) => void submit(event)}>
        <p className="text-sm leading-6 text-ink/70 dark:text-white/70">{message}</p>
        <label className="mt-5 block">
          <span className="text-sm font-semibold">{label}</span>
          <input
            required
            minLength={minLength}
            maxLength={maxLength}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            className="mt-1 min-h-11 w-full rounded-xl border border-ink/15 bg-transparent px-3 py-2 text-sm outline-none focus:border-brand dark:border-white/20"
          />
        </label>
        <p className="mt-2 text-xs text-ink/50 dark:text-white/50">
          {invalid ? `Use between ${minLength} and ${maxLength} characters.` : `${trimmed.length} of ${maxLength} characters.`}
        </p>
        <div className="mt-7 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <button type="button" onClick={onCancel} className={cancelClass}>Cancel</button>
          <button type="submit" disabled={busy || invalid || trimmed === initialValue} className={primaryClass}>{confirmLabel}</button>
        </div>
      </form>
    </Modal>
  );
}
