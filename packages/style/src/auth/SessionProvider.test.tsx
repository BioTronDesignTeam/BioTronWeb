import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useSession } from './session';
import { SessionProvider } from './SessionProvider';
import { authStrings } from './strings';

const authUrl = 'https://auth.example.test';
const octocat = { github_id: 1, login: 'octocat', name: 'Octocat', avatar_url: '' };

type Reply = { status: number; body?: unknown } | 'network';
type Route = (url: URL, init?: RequestInit) => Reply;

// A fake Auth: each call is answered by `route`, and every call is recorded.
function fakeAuth(route: Route) {
  const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
    const reply = route(new URL(input), init);
    if (reply === 'network') throw new TypeError('Failed to fetch');
    return new Response(reply.body === undefined ? null : JSON.stringify(reply.body), {
      status: reply.status,
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderSession(props: { appId?: string; permissions?: readonly string[] } = {}) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <SessionProvider authUrl={`${authUrl}/`} {...props}>
      {children}
    </SessionProvider>
  );
  return renderHook(() => useSession(), { wrapper });
}

// Lets every pending fetch, retry wait, and state update settle.
async function settle(ms = 5000) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  window.history.replaceState(null, '', '/');
});

afterEach(() => {
  // Without Vitest globals, Testing Library does not unmount on its own.
  cleanup();
  vi.useRealTimers();
});

describe('SessionProvider', () => {
  it('starts loading, then signs in with the permissions it was asked for', async () => {
    const fetchMock = fakeAuth((url) => {
      if (url.pathname === '/auth/me') return { status: 200, body: octocat };
      return { status: 200, body: { allowed: url.searchParams.get('permission') === 'write' } };
    });
    const { result } = renderSession({ appId: 'calendar', permissions: ['write', 'admin'] });
    expect(result.current.status).toBe('loading');

    await settle();
    expect(result.current.status).toBe('authed');
    expect(result.current.operator?.login).toBe('octocat');
    expect(result.current.permissions).toEqual({ write: true, admin: false });
    expect(result.current.can('write')).toBe(true);
    expect(result.current.can('admin')).toBe(false);
    expect(result.current.can(['admin', 'write'])).toBe(true);

    const me = fetchMock.mock.calls[0];
    expect(me?.[0]).toBe(`${authUrl}/auth/me?app=calendar`);
    expect(me?.[1]).toMatchObject({ credentials: 'include' });
  });

  it('is signed out on a 401 or a 403', async () => {
    for (const status of [401, 403]) {
      fakeAuth(() => ({ status }));
      const { result, unmount } = renderSession({ appId: 'exo-gui' });
      await settle();
      expect(result.current.status).toBe('anon');
      expect(result.current.notice).toBeNull();
      unmount();
    }
  });

  it('is signed out when a permission check answers 401', async () => {
    fakeAuth((url) =>
      url.pathname === '/auth/me' ? { status: 200, body: octocat } : { status: 401 },
    );
    const { result } = renderSession({ appId: 'calendar', permissions: ['write'] });
    await settle();
    expect(result.current.status).toBe('anon');
  });

  it('retries, then says Auth is unavailable rather than signed out', async () => {
    const fetchMock = fakeAuth(() => ({ status: 502 }));
    const { result } = renderSession();
    await settle(400);
    expect(result.current.status).toBe('loading');
    await settle();
    expect(result.current.status).toBe('unavailable');
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it('treats a network failure as an outage', async () => {
    fakeAuth(() => 'network');
    const { result } = renderSession();
    await settle();
    expect(result.current.status).toBe('unavailable');
  });

  it('keeps retrying while Auth is unavailable and recovers', async () => {
    let up = false;
    fakeAuth(() => (up ? { status: 200, body: octocat } : { status: 503 }));
    const { result } = renderSession();
    await settle();
    expect(result.current.status).toBe('unavailable');

    up = true;
    await settle(15_000);
    expect(result.current.status).toBe('authed');
  });

  it('keeps a live session through an outage', async () => {
    let status = 200;
    fakeAuth(() => ({ status, body: octocat }));
    const { result } = renderSession();
    await settle();
    expect(result.current.status).toBe('authed');

    status = 500;
    act(() => void result.current.refresh());
    await settle();
    expect(result.current.status).toBe('authed');
    expect(result.current.operator?.login).toBe('octocat');
  });

  it('says the session expired when a live one is refused', async () => {
    let status = 200;
    fakeAuth(() => ({ status, body: octocat }));
    const { result } = renderSession();
    await settle();

    status = 401;
    act(() => result.current.expire());
    await settle();
    expect(result.current.status).toBe('anon');
    expect(result.current.notice).toBe('expired');

    act(() => result.current.dismissNotice());
    expect(result.current.notice).toBeNull();
  });

  it('reads ?auth=denied once and strips it from the address', async () => {
    window.history.replaceState(null, '', '/page?auth=denied&tab=2#top');
    fakeAuth(() => ({ status: 401 }));
    const { result } = renderSession();
    expect(result.current.notice).toBe('denied');
    expect(`${window.location.pathname}${window.location.search}${window.location.hash}`).toBe(
      '/page?tab=2#top',
    );
    await settle();
    expect(result.current.notice).toBe('denied');
  });

  it('signs out through a POST that carries the CSRF header', async () => {
    const fetchMock = fakeAuth((url) =>
      url.pathname === '/auth/logout' ? { status: 204 } : { status: 200, body: octocat },
    );
    const { result } = renderSession();
    await settle();

    await act(async () => {
      await result.current.logout();
    });
    expect(result.current.status).toBe('anon');
    const logout = fetchMock.mock.calls.find(([input]) => input.endsWith('/auth/logout'));
    expect(logout?.[1]).toMatchObject({
      method: 'POST',
      credentials: 'include',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    });
  });

  it('counts a 401 on sign-out as signed out', async () => {
    fakeAuth((url) =>
      url.pathname === '/auth/logout' ? { status: 401 } : { status: 200, body: octocat },
    );
    const { result } = renderSession();
    await settle();
    await act(async () => {
      await result.current.logout();
    });
    expect(result.current.status).toBe('anon');
  });

  it('keeps the session when sign-out fails', async () => {
    fakeAuth((url) =>
      url.pathname === '/auth/logout' ? { status: 500 } : { status: 200, body: octocat },
    );
    const { result } = renderSession();
    await settle();
    await act(async () => {
      await expect(result.current.logout()).rejects.toThrow(authStrings.signOutFailed);
    });
    expect(result.current.status).toBe('authed');
  });

  it('accepts a daily guest key only for an app', async () => {
    let guest = false;
    const fetchMock = fakeAuth((url) => {
      if (url.pathname === '/auth/guest') {
        guest = true;
        return { status: 204 };
      }
      return guest ? { status: 200, body: { ...octocat, is_guest: true } } : { status: 401 };
    });
    const withoutApp = renderSession();
    await settle();
    expect(await withoutApp.result.current.loginGuest('KEY')).toBe(false);
    withoutApp.unmount();

    const { result } = renderSession({ appId: 'exo-gui' });
    await settle();
    let accepted = false;
    await act(async () => {
      accepted = await result.current.loginGuest('ABCD-EFGH-IJKL');
    });
    expect(accepted).toBe(true);
    expect(result.current.status).toBe('authed');
    expect(result.current.operator?.is_guest).toBe(true);
    const post = fetchMock.mock.calls.find(([input]) => input.endsWith('/auth/guest'));
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({
      app_id: 'exo-gui',
      key: 'ABCD-EFGH-IJKL',
    });
  });

  it('refuses a wrong guest key', async () => {
    fakeAuth((url) => (url.pathname === '/auth/guest' ? { status: 403 } : { status: 401 }));
    const { result } = renderSession({ appId: 'exo-gui' });
    await settle();
    let accepted = true;
    await act(async () => {
      accepted = await result.current.loginGuest('WRONG');
    });
    expect(accepted).toBe(false);
    expect(result.current.status).toBe('anon');
  });
});
