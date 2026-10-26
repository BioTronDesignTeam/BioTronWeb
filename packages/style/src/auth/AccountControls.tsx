import { useState } from 'react';
import { Button } from '../Button';
import { ThemeToggle } from '../ThemeToggle';
import { UserMenu } from '../UserMenu';
import { useSession } from './session';
import { authStrings, noticeMessage } from './strings';
import type { SessionOperator } from './session';

export type AccountControlsProps = {
  /** The line under the name in the menu. Defaults to `@login`. */
  detail?: (operator: SessionOperator) => string;
  /** Runs after Auth confirms the sign-out, to drop page state. */
  onSignedOut?: () => void;
  showThemeToggle?: boolean;
  className?: string;
};

/**
 * The right-hand end of a tool header: the theme switch, then the account
 * menu when signed in or a Sign in button when not. It also shows a sign-out
 * failure and, on a page that stays public, the denied, banned, or expired
 * notice as a dismissible toast.
 */
export function AccountControls({ detail, onSignedOut, showThemeToggle = true, className = '' }: AccountControlsProps) {
  const { status, operator, login, logout, refresh, notice, dismissNotice } = useSession();
  const [error, setError] = useState('');

  const signOut = async () => {
    setError('');
    try {
      await logout();
      onSignedOut?.();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : authStrings.signOutFailed);
    }
  };

  const toast = error || (notice && status !== 'authed' ? noticeMessage(notice) : '');
  const closeToast = () => {
    if (error) setError('');
    else dismissNotice();
  };

  return (
    <div className={`biotron-account-controls ${className}`.trim()}>
      {showThemeToggle && <ThemeToggle />}
      {toast && (
        <div className="biotron-toast" role="alert">
          <span>{toast}</span>
          <button type="button" className="biotron-toast__close" onClick={closeToast} aria-label={authStrings.dismiss}>×</button>
        </div>
      )}
      {status === 'authed' && operator ? (
        <UserMenu
          user={{
            name: operator.name,
            login: operator.login,
            avatarUrl: operator.avatar_url,
            detail: detail ? detail(operator) : undefined,
          }}
          onLogout={signOut}
        />
      ) : status === 'unavailable' ? (
        <Button tone="neutral" onClick={() => void refresh()} className="biotron-account-controls__button" title={authStrings.unavailable}>
          {authStrings.unavailableShort}
        </Button>
      ) : status === 'anon' ? (
        <Button onClick={login} className="biotron-account-controls__button">{authStrings.signIn}</Button>
      ) : null}
    </div>
  );
}
