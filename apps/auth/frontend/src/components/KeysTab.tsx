import { useEffect, useRef, useState } from 'react';
import type { ProductDailyKey } from '../api';
import {
  compactSecondaryButton,
  dangerButton,
  divided,
  mutedText,
  panel,
  panelEmpty,
  panelHeader,
  panelHeading,
  primaryButton,
  secondaryButton,
} from './ui';

const hiddenKey = '••••-••••-••••';

export default function KeysTab({
  dailyKeys,
  revealedAppIds,
  copiedAppId,
  onToggle,
  onCopy,
  onRotate,
}: {
  dailyKeys: ProductDailyKey[];
  revealedAppIds: ReadonlySet<string>;
  copiedAppId: string | null;
  onToggle: (appId: string) => void;
  onCopy: (key: ProductDailyKey) => void;
  onRotate: (appId: string) => void;
}) {
  const [pending, setPending] = useState<ProductDailyKey | null>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);

  // The same native dialog as a ban: rotating signs every guest of the
  // product out, so it asks first.
  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (pending && !dialog.open) dialog.showModal();
    else if (!pending && dialog.open) dialog.close();
  }, [pending]);

  const rotate = () => {
    if (!pending) return;
    const appId = pending.app_id;
    setPending(null);
    onRotate(appId);
  };

  return (
    <section className={panel}>
      <div className={panelHeader}>
        <h2 className={panelHeading}>Daily guest keys</h2>
        <p className={`mt-1 ${mutedText}`}>
          Each tool that admits guests has its own daily guest key, which rotates at Eastern midnight.
          A guest who signs in with a tool&apos;s key gets that tool&apos;s Live and
          Historical permissions until the key rotates, and nothing else. Rotate a key early to
          sign out every guest who used it.
        </p>
      </div>
      {dailyKeys.length === 0 ? (
        <p className={panelEmpty}>No tool uses a daily guest key.</p>
      ) : (
        <ul className={divided}>
          {dailyKeys.map((key) => {
            const revealed = revealedAppIds.has(key.app_id);
            return (
              <li
                key={key.app_id}
                className="flex flex-col gap-4 px-4 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6"
              >
                <div className="min-w-0">
                  <h3 className="text-base font-medium text-slate-900 dark:text-white">
                    {key.app_name}
                  </h3>
                  <p className={`mt-1 ${mutedText}`}>Valid for {key.day} in America/Toronto</p>
                  <p
                    // The key is 14 characters of wide monospace; it only fits a
                    // 375px column once the tracking and size come down.
                    className="mt-3 font-mono text-base tracking-[0.12em] break-all text-slate-900 sm:text-lg sm:tracking-[0.16em] dark:text-white"
                    aria-label={
                      revealed ? `${key.app_name} daily guest key ${key.key}` : `${key.app_name} daily guest key hidden`
                    }
                  >
                    {revealed ? key.key : hiddenKey}
                  </p>
                </div>
                <div className="grid grid-cols-2 gap-2 sm:flex">
                  <button type="button" onClick={() => onToggle(key.app_id)} className={secondaryButton}>
                    {revealed ? 'Hide' : 'Reveal'}
                  </button>
                  <button type="button" onClick={() => onCopy(key)} className={primaryButton}>
                    {copiedAppId === key.app_id ? 'Copied' : 'Copy'}
                  </button>
                  <button
                    type="button"
                    onClick={() => setPending(key)}
                    className={`col-span-2 ${dangerButton}`}
                  >
                    Rotate…
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
      <dialog
        ref={dialogRef}
        onClose={() => setPending(null)}
        aria-labelledby="rotate-key-title"
        aria-describedby="rotate-key-detail"
        className="m-auto w-full max-w-md rounded-xl border border-slate-300 bg-slate-50 p-4 text-left backdrop:bg-black/40 dark:border-white/15 dark:bg-surface-2"
      >
        {pending && (
          <>
            <p id="rotate-key-title" className="text-sm font-semibold text-slate-900 dark:text-white">
              Rotate today&apos;s {pending.app_name} daily guest key?
            </p>
            <p id="rotate-key-detail" className={`mt-1 ${mutedText}`}>
              Every guest signed in with the current key is signed out at their next request, and
              the current key stops working. Share the new key with the guests who should keep using {pending.app_name}.
            </p>
            <div className="mt-3 flex flex-wrap gap-2">
              <button type="button" onClick={rotate} className={dangerButton}>
                Rotate
              </button>
              <button type="button" onClick={() => setPending(null)} className={compactSecondaryButton}>
                Cancel
              </button>
            </div>
          </>
        )}
      </dialog>
    </section>
  );
}
