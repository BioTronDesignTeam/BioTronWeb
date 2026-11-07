import { authStrings } from '@biotron/style';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  API_BASE,
  banMember,
  listOrgMembers,
  setManager,
  setUnauthorizedHandler,
  signOutMember,
} from './api';

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

describe('auth api', () => {
  it('has no trailing slash on its base', () => {
    expect(API_BASE.endsWith('/')).toBe(false);
  });

  it('sends a read with the cookie and no CSRF header', async () => {
    const call = answer(200, []);
    await listOrgMembers();
    const [url, init] = call();
    expect(url).toBe(`${API_BASE}/org/members`);
    expect(init.credentials).toBe('include');
    expect(new Headers(init.headers).has('X-Requested-With')).toBe(false);
  });

  it('sends a change with the cookie, the CSRF header, and JSON', async () => {
    const call = answer(204);
    await setManager(7, true);
    const [url, init] = call();
    expect(url).toBe(`${API_BASE}/org/members/7/manager`);
    expect(init).toMatchObject({
      method: 'PATCH',
      credentials: 'include',
      body: '{"manager":true}',
    });
    const headers = new Headers(init.headers);
    expect(headers.get('X-Requested-With')).toBe('XMLHttpRequest');
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('signs a member out of every device with a CSRF-guarded POST', async () => {
    const call = answer(204);
    await expect(signOutMember(7)).resolves.toBeUndefined();
    const [url, init] = call();
    expect(url).toBe(`${API_BASE}/org/members/7/sign-out`);
    expect(init).toMatchObject({ method: 'POST', credentials: 'include' });
    expect(new Headers(init.headers).get('X-Requested-With')).toBe('XMLHttpRequest');
  });

  it('shows the refusals of a sign-out as Auth words them', async () => {
    answer(400, { error: "You can't sign yourself out here." });
    await expect(signOutMember(1)).rejects.toThrow("You can't sign yourself out here.");
    answer(403, { error: 'Only a superuser can sign out a superuser.' });
    await expect(signOutMember(2)).rejects.toThrow('Only a superuser can sign out a superuser.');
  });

  it('tells the handler once about a 401', async () => {
    answer(401, { error: 'unauthorized' });
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    await expect(banMember(3)).rejects.toThrow(authStrings.expired);
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('turns a bare 403 and a 503 into the shared sentences', async () => {
    answer(403, { error: 'permission required' });
    await expect(listOrgMembers()).rejects.toThrow(authStrings.forbidden);
    answer(503);
    await expect(listOrgMembers()).rejects.toThrow(authStrings.serviceUnavailable);
  });

  it('returns nothing for a 204', async () => {
    answer(204);
    await expect(banMember(3)).resolves.toBeUndefined();
  });
});
