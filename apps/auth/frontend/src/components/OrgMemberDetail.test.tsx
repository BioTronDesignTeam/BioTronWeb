// @vitest-environment jsdom
import { authStrings } from '@biotron/style';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import type { OrgMember } from '../api';
import OrgMemberDetail from './OrgMemberDetail';

beforeAll(() => {
  // jsdom has <dialog> without its modal methods; opening and closing is
  // all these tests need of them.
  const dialog = HTMLDialogElement.prototype;
  if (!('showModal' in dialog)) {
    Object.defineProperty(dialog, 'showModal', {
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '');
      },
    });
  }
  if (!('close' in dialog)) {
    Object.defineProperty(dialog, 'close', {
      value(this: HTMLDialogElement) {
        this.removeAttribute('open');
      },
    });
  }
});

afterEach(() => cleanup());

const sam: OrgMember = {
  github_id: 2,
  login: 'sam',
  name: 'Sam Lee',
  avatar_url: '',
  is_superuser: false,
  is_manager: false,
  is_banned: false,
  last_login_at: '2026-11-07T12:00:00Z',
};

function detail(member: OrgMember, { meId = 1, canManage = false } = {}) {
  const onSignOut = vi.fn();
  render(
    <OrgMemberDetail
      member={member}
      meId={meId}
      apps={[]}
      permissions={[]}
      grants={[]}
      selection={{ memberId: member.github_id, appId: '', permissionKey: '' }}
      canManage={canManage}
      onSelectApp={vi.fn()}
      onSelectPermission={vi.fn()}
      onCreateGrant={vi.fn()}
      onRevokeGrant={vi.fn()}
      onBan={vi.fn()}
      onUnban={vi.fn()}
      onSignOut={onSignOut}
      onSetManager={vi.fn()}
    />,
  );
  return onSignOut;
}

const action = `${authStrings.signOutMember}…`;

describe('Sign out of all devices', () => {
  it('asks first, then signs the member out', () => {
    const onSignOut = detail(sam);
    fireEvent.click(screen.getByRole('button', { name: action }));
    expect(onSignOut).not.toHaveBeenCalled();
    expect(screen.getByText(authStrings.signOutMemberTitle('@sam'))).toBeTruthy();
    expect(screen.getByText(authStrings.signOutMemberDetail)).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: authStrings.signOutMember }));
    expect(onSignOut).toHaveBeenCalledWith(sam);
  });

  it('does nothing when the confirmation is cancelled', () => {
    const onSignOut = detail(sam);
    fireEvent.click(screen.getByRole('button', { name: action }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onSignOut).not.toHaveBeenCalled();
  });

  it('is not offered for yourself', () => {
    detail(sam, { meId: sam.github_id });
    expect(screen.queryByRole('button', { name: action })).toBeNull();
  });

  it('is offered for a superuser only to a superuser', () => {
    const root = { ...sam, is_superuser: true };
    detail(root, { canManage: false });
    expect(screen.queryByRole('button', { name: action })).toBeNull();
    cleanup();
    detail(root, { canManage: true });
    expect(screen.getByRole('button', { name: action })).toBeTruthy();
  });
});
