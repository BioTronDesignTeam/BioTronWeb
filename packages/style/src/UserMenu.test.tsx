import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AccountControls } from './auth/AccountControls';
import { SessionContext, type SessionOperator, type SessionValue } from './auth/session';
import { authStrings } from './auth/strings';
import { UserMenu } from './UserMenu';

afterEach(() => cleanup());

function openMenu() {
  const trigger = screen.getByRole('button', { name: authStrings.userMenu });
  fireEvent.click(trigger);
  return trigger;
}

describe('UserMenu', () => {
  it('is a disclosure: a button that shows a panel of ordinary buttons', () => {
    render(<UserMenu user={{ name: 'Octocat', login: 'octocat' }} onLogout={vi.fn()} />);
    const trigger = openMenu();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(trigger.hasAttribute('aria-haspopup')).toBe(false);
    expect(screen.queryByRole('menu')).toBeNull();
    const panel = document.getElementById(trigger.getAttribute('aria-controls') ?? '');
    expect(panel?.textContent).toContain('Octocat');
    expect(screen.getByRole('button', { name: authStrings.signOut })).toBeTruthy();
  });

  it('closes on Escape and puts focus back on its button', () => {
    render(<UserMenu onLogout={vi.fn()} />);
    const trigger = openMenu();
    screen.getByRole('button', { name: authStrings.signOut }).focus();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('button', { name: authStrings.signOut })).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('closes when focus leaves it', () => {
    render(
      <>
        <UserMenu onLogout={vi.fn()} />
        <button type="button">Elsewhere</button>
      </>,
    );
    const trigger = openMenu();
    act(() => screen.getByRole('button', { name: 'Elsewhere' }).focus());
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it('offers Sign out everywhere only when given the action', () => {
    const everywhere = vi.fn();
    const { unmount } = render(<UserMenu onLogout={vi.fn()} />);
    openMenu();
    expect(screen.queryByRole('button', { name: authStrings.signOutEverywhere })).toBeNull();
    unmount();

    render(<UserMenu onLogout={vi.fn()} onLogoutEverywhere={everywhere} />);
    openMenu();
    fireEvent.click(screen.getByRole('button', { name: authStrings.signOutEverywhere }));
    expect(everywhere).toHaveBeenCalledTimes(1);
  });
});

function signedIn(operator: SessionOperator, overrides: Partial<SessionValue> = {}): SessionValue {
  return {
    status: 'authed',
    operator,
    permissions: {},
    can: () => false,
    notice: null,
    dismissNotice: vi.fn(),
    login: vi.fn(),
    loginGuest: vi.fn(async () => false),
    logout: vi.fn(async () => undefined),
    logoutEverywhere: vi.fn(async () => undefined),
    refresh: vi.fn(async () => undefined),
    expire: vi.fn(),
    ...overrides,
  };
}

const member = { github_id: 1, login: 'octocat', name: 'Octocat', avatar_url: '' };

function controls(value: SessionValue, onSignedOut = vi.fn()) {
  render(
    <SessionContext.Provider value={value}>
      <AccountControls showThemeToggle={false} onSignedOut={onSignedOut} />
    </SessionContext.Provider>,
  );
  return onSignedOut;
}

describe('AccountControls', () => {
  it('signs a member out everywhere from the menu', async () => {
    const value = signedIn(member);
    const onSignedOut = controls(value);
    openMenu();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: authStrings.signOutEverywhere }));
    });
    expect(value.logoutEverywhere).toHaveBeenCalledTimes(1);
    expect(value.logout).not.toHaveBeenCalled();
    expect(onSignedOut).toHaveBeenCalledTimes(1);
  });

  it('does not offer Sign out everywhere to a guest', () => {
    controls(signedIn({ ...member, is_guest: true, guest_app_id: 'exo-gui' }));
    openMenu();
    expect(screen.getByRole('button', { name: authStrings.signOut })).toBeTruthy();
    expect(screen.queryByRole('button', { name: authStrings.signOutEverywhere })).toBeNull();
  });

  it('says so when signing out everywhere fails', async () => {
    const value = signedIn(member, {
      logoutEverywhere: vi.fn(async () => {
        throw new Error(authStrings.signOutEverywhereFailed);
      }),
    });
    const onSignedOut = controls(value);
    openMenu();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: authStrings.signOutEverywhere }));
    });
    expect(screen.getByRole('alert').textContent).toContain(authStrings.signOutEverywhereFailed);
    expect(onSignedOut).not.toHaveBeenCalled();
  });
});
