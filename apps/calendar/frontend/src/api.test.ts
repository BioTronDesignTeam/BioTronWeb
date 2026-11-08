import { authStrings } from '@biotron/style';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { API_URL, calendarApi, setUnauthorizedHandler } from './api';

type Call = [string, RequestInit];

// Answers every request with `status` and `body`, and records it.
function answer(status: number, body?: unknown) {
  const fetchMock = vi.fn(
    async (_input: string, _init?: RequestInit) =>
      new Response(body === undefined ? null : JSON.stringify(body), { status }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return () => fetchMock.mock.calls[0] as Call;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setUnauthorizedHandler(() => undefined);
});

describe('calendar request', () => {
  it('sends a public read without the cookie or the CSRF header', async () => {
    const call = answer(200, []);
    await calendarApi.scopes();
    const [url, init] = call();
    expect(url).toBe(`${API_URL}/v1/scopes`);
    expect(init.credentials).toBe('omit');
    expect(new Headers(init.headers).has('X-Requested-With')).toBe(false);
  });

  it('sends an editor change with the cookie, the CSRF header, and JSON', async () => {
    const call = answer(200, { id: 's' });
    await calendarApi.renameScope('s', 'Exo');
    const [url, init] = call();
    expect(url).toBe(`${API_URL}/v1/admin/scopes/s`);
    expect(init).toMatchObject({ method: 'PATCH', credentials: 'include' });
    const headers = new Headers(init.headers);
    expect(headers.get('X-Requested-With')).toBe('XMLHttpRequest');
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('sends an editor read with the cookie and no CSRF header', async () => {
    const call = answer(200, []);
    await calendarApi.adminEvents();
    const [, init] = call();
    expect(init.credentials).toBe('include');
    expect(new Headers(init.headers).has('X-Requested-With')).toBe(false);
  });

  it('tells the handler once about a 401 on an editor request', async () => {
    answer(401, { error: 'unauthorized' });
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    await expect(calendarApi.adminScopes()).rejects.toThrow(authStrings.expired);
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('does not report a 401 on a public read as an expired session', async () => {
    answer(401);
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    await expect(calendarApi.scopes()).rejects.toThrow();
    expect(handler).not.toHaveBeenCalled();
  });

  it('turns 403 and 503 into the shared sentences', async () => {
    answer(403, { error: 'permission required' });
    await expect(calendarApi.deleteScope('s')).rejects.toThrow(authStrings.forbidden);
    answer(503, { error: 'auth unavailable' });
    await expect(calendarApi.deleteScope('s')).rejects.toThrow(authStrings.serviceUnavailable);
  });

  it('keeps a specific message from the server', async () => {
    answer(409, { error: 'A calendar with that name already exists.' });
    await expect(calendarApi.renameScope('s', 'Exo')).rejects.toThrow(
      'A calendar with that name already exists.',
    );
  });

  it('returns nothing for a 204', async () => {
    answer(204);
    await expect(calendarApi.deleteEvent('e')).resolves.toBeUndefined();
  });
});
