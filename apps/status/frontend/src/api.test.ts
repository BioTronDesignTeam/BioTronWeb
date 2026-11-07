import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIError, getStatus, getStatusHistory } from './api';

type Call = [string, RequestInit];

// Answers every request with `status` and a raw `body`, and records it.
function answer(status: number, body?: string) {
  const fetchMock = vi.fn(
    async (_input: string, _init?: RequestInit) => new Response(body ?? null, { status }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return () => fetchMock.mock.calls[0] as Call;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('status api', () => {
  it('reads without the cookie or the CSRF header, the page being public', async () => {
    const call = answer(200, '{}');
    await getStatus();
    const [url, init] = call();
    expect(url).toBe('/api/v1/status');
    expect(init.credentials).toBe('omit');
    expect(new Headers(init.headers).has('X-Requested-With')).toBe(false);
  });

  it('asks for the number of history days', async () => {
    const call = answer(200, '{}');
    await getStatusHistory(30);
    expect(call()[0]).toBe('/api/v1/status/history?days=30');
  });

  it('keeps the status and the message of a refusal', async () => {
    answer(503, '{"error":"monitor starting"}');
    const error = await getStatus().catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(APIError);
    expect(error).toMatchObject({ status: 503, message: 'monitor starting' });
  });

  it('falls back to the status when a proxy answers in plain text', async () => {
    answer(502, 'Bad Gateway');
    await expect(getStatus()).rejects.toMatchObject({ status: 502, message: 'HTTP 502' });
  });
});
