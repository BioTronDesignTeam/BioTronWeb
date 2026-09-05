import type { ProductDailyKey } from '../api';
import {
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
}: {
  dailyKeys: ProductDailyKey[];
  revealedAppIds: ReadonlySet<string>;
  copiedAppId: string | null;
  onToggle: (appId: string) => void;
  onCopy: (key: ProductDailyKey) => void;
}) {
  return (
    <section className={panel}>
      <div className={panelHeader}>
        <h2 className={panelHeading}>Daily product keys</h2>
        <p className={`mt-1 ${mutedText}`}>
          Each enabled product receives its own key. Keys rotate independently at Eastern midnight.
          A guest who signs in with a product&apos;s key gets that product&apos;s Live and
          Historical permissions until the key rotates, and nothing else.
        </p>
      </div>
      {dailyKeys.length === 0 ? (
        <p className={panelEmpty}>No products currently use daily keys.</p>
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
                      revealed ? `${key.app_name} daily key ${key.key}` : `${key.app_name} daily key hidden`
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
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
