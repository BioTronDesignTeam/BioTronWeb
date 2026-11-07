import { cleanup, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RequireAuth } from './screens';
import { SessionContext, type SessionValue } from './session';
import { authStrings } from './strings';

const octocat = { github_id: 1, login: 'octocat', name: 'Octocat', avatar_url: '' };

// A session in a given state, holding the listed permissions.
function session(status: SessionValue['status'], granted: string[] = []): SessionValue {
  return {
    status,
    operator: status === 'authed' ? octocat : null,
    permissions: Object.fromEntries(granted.map((key) => [key, true])),
    can: (permission) =>
      (typeof permission === 'string' ? [permission] : permission).some((key) =>
        granted.includes(key),
      ),
    notice: null,
    dismissNotice: vi.fn(),
    login: vi.fn(),
    loginGuest: vi.fn(async () => false),
    logout: vi.fn(async () => undefined),
    logoutEverywhere: vi.fn(async () => undefined),
    refresh: vi.fn(async () => undefined),
    expire: vi.fn(),
  };
}

function gate(
  value: SessionValue,
  props: { permission?: string | string[]; guest?: boolean } = {},
) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
  return render(
    <RequireAuth productName="Exo GUI" {...props}>
      <p>The app</p>
    </RequireAuth>,
    { wrapper },
  );
}

afterEach(() => cleanup());

describe('RequireAuth', () => {
  it('shows Loading while the first check runs', () => {
    gate(session('loading'));
    expect(screen.getByText(authStrings.loading)).toBeTruthy();
    expect(screen.queryByText('The app')).toBeNull();
  });

  it('shows the outage screen with a retry when Auth is unavailable', () => {
    gate(session('unavailable'));
    expect(screen.getByText(authStrings.unavailable)).toBeTruthy();
    expect(screen.getByRole('button', { name: authStrings.retry })).toBeTruthy();
    expect(screen.queryByText('The app')).toBeNull();
  });

  it('asks a signed-out visitor to sign in, with no guest form by default', () => {
    gate(session('anon'));
    expect(screen.getByRole('button', { name: authStrings.signInWithGitHub })).toBeTruthy();
    expect(screen.queryByLabelText(authStrings.guestKeyLabel)).toBeNull();
    expect(screen.queryByText('The app')).toBeNull();
  });

  it('offers the daily guest key when asked to', () => {
    gate(session('anon'), { guest: true });
    expect(screen.getByLabelText(authStrings.guestKeyLabel)).toBeTruthy();
  });

  it('says Access denied to a signed-in person without the permission', () => {
    gate(session('authed', ['historical']), { permission: ['live', 'commands'] });
    expect(screen.getByText(authStrings.accessDenied)).toBeTruthy();
    expect(screen.getByText(authStrings.signedInAs('octocat'))).toBeTruthy();
    expect(screen.queryByText('The app')).toBeNull();
  });

  it('shows the page for any one of the listed permissions', () => {
    gate(session('authed', ['commands']), { permission: ['live', 'historical', 'commands'] });
    expect(screen.getByText('The app')).toBeTruthy();
  });

  it('shows the page to anyone signed in when no permission is named', () => {
    gate(session('authed'));
    expect(screen.getByText('The app')).toBeTruthy();
  });
});
