import { useCallback, useState } from 'react';
import { type ProductDailyKey } from '../api';

/**
 * Reveal and copy state for the Keys tab. It lives above the tab so a key you
 * revealed is still revealed when you come back from another tab. Signing out
 * unmounts the console, which drops it.
 */
export function useDailyKeyReveal(onError: (message: string) => void) {
  const [revealedAppIds, setRevealedAppIds] = useState<ReadonlySet<string>>(() => new Set());
  const [copiedAppId, setCopiedAppId] = useState<string | null>(null);

  const toggle = useCallback((appId: string) => {
    setRevealedAppIds((current) => {
      const next = new Set(current);
      if (next.has(appId)) next.delete(appId);
      else next.add(appId);
      return next;
    });
  }, []);

  const copy = useCallback(
    async (key: ProductDailyKey) => {
      try {
        await navigator.clipboard.writeText(key.key);
        setCopiedAppId(key.app_id);
        window.setTimeout(
          () => setCopiedAppId((current) => (current === key.app_id ? null : current)),
          2000,
        );
      } catch {
        onError('Could not copy key');
      }
    },
    [onError],
  );

  return { revealedAppIds, copiedAppId, toggle, copy };
}
