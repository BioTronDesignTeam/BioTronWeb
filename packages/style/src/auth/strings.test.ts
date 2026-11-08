import { describe, expect, it } from 'vitest';
import {
  authStrings,
  friendlyError,
  identityLine,
  isSessionExpired,
  noticeMessage,
  roleOf,
} from './strings';

describe('roleOf', () => {
  it('names one role, highest first', () => {
    expect(roleOf({})).toBe('Member');
    expect(roleOf({ is_manager: true })).toBe('Manager');
    expect(roleOf({ is_superuser: true, is_manager: true })).toBe('Superuser');
  });

  it('calls a guest a guest whatever else is set', () => {
    expect(roleOf({ is_guest: true, is_superuser: true, is_manager: true })).toBe('Guest');
  });
});

describe('identityLine', () => {
  it('puts the role after the login', () => {
    expect(identityLine({ login: 'octocat', is_manager: true })).toBe('@octocat · Manager');
    expect(identityLine({ login: 'guest-1', is_guest: true })).toBe('@guest-1 · Guest');
  });
});

describe('friendlyError', () => {
  it('says the session expired on any 401', () => {
    expect(friendlyError(401)).toBe(authStrings.expired);
    expect(friendlyError(401, 'session revoked')).toBe(authStrings.expired);
  });

  it('replaces the terse 403 reasons backends send', () => {
    for (const reason of [
      undefined,
      '',
      'HTTP 403',
      'permission required',
      ' Permission Required ',
      'X-Requested-With header required',
    ]) {
      expect(friendlyError(403, reason)).toBe(authStrings.forbidden);
    }
  });

  it('keeps a specific 403 message', () => {
    expect(friendlyError(403, 'This scope is archived.')).toBe('This scope is archived.');
  });

  it('says Auth is unreachable on a 503', () => {
    expect(friendlyError(503, 'auth unavailable')).toBe(authStrings.serviceUnavailable);
  });

  it('turns an empty or generic message into a sentence with the status', () => {
    expect(friendlyError(500)).toBe('Something went wrong (500). Try again.');
    expect(friendlyError(502, '  ')).toBe('Something went wrong (502). Try again.');
    expect(friendlyError(500, 'HTTP 500')).toBe('Something went wrong (500). Try again.');
    expect(friendlyError(404, 'Request failed (404)')).toBe(
      'Something went wrong (404). Try again.',
    );
  });

  it('keeps any other message the server sent, trimmed', () => {
    expect(friendlyError(400, '  Ends must be after starts.  ')).toBe('Ends must be after starts.');
  });
});

describe('isSessionExpired', () => {
  it('is true only for 401', () => {
    expect(isSessionExpired(401)).toBe(true);
    expect(isSessionExpired(403)).toBe(false);
  });
});

describe('noticeMessage', () => {
  it('reads each sign-in notice from authStrings', () => {
    expect(noticeMessage('denied')).toBe(authStrings.denied);
    expect(noticeMessage('banned')).toBe(authStrings.banned);
    expect(noticeMessage('expired')).toBe(authStrings.expired);
  });
});
