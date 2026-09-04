import { useEffect, useState } from 'react';

export type AuthNotice = 'denied' | 'banned' | null;

function readAuthNotice(): AuthNotice {
  const value = new URLSearchParams(window.location.search).get('auth');
  return value === 'denied' || value === 'banned' ? value : null;
}

/**
 * The GitHub callback bounces non-members back with `?auth=denied` and banned
 * operators with `?auth=banned`. The value is read while the first render is
 * being computed — initialising it from a mount effect would paint one frame
 * of the sign-in screen with no explanation — and the query string is stripped
 * afterwards so a reload does not resurrect a stale notice.
 */
export function useAuthNotice(): AuthNotice {
  const [notice] = useState<AuthNotice>(readAuthNotice);

  useEffect(() => {
    if (!notice) return;
    window.history.replaceState({}, '', window.location.pathname);
  }, [notice]);

  return notice;
}
