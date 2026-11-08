import { useEffect, useId, useRef, useState } from 'react';
import { authStrings } from './auth/strings';

const UserIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M12 12a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0 2c-4.42 0-8 2.69-8 6v1a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-1c0-3.31-3.58-6-8-6Z" />
  </svg>
);

export type UserMenuIdentity = {
  name?: string;
  login?: string;
  avatarUrl?: string;
  detail?: string;
};

export type UserMenuProps = {
  user?: UserMenuIdentity;
  onLogout: () => void | Promise<void>;
  logoutLabel?: string;
  /** Adds a "Sign out everywhere" button, which ends every session of this person. */
  onLogoutEverywhere?: () => void | Promise<void>;
  className?: string;
};

/**
 * The account button and the panel it opens. It is a disclosure, not an ARIA
 * menu: a button that shows a panel of ordinary buttons, reached with Tab.
 * Escape, or Tab out of the panel, closes it; Escape puts focus back on the
 * button.
 */
export function UserMenu({
  user,
  onLogout,
  logoutLabel = authStrings.signOut,
  onLogoutEverywhere,
  className = '',
}: UserMenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setOpen(false);
      trigger.current?.focus();
    };
    const onFocusIn = (event: FocusEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('focusin', onFocusIn);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('focusin', onFocusIn);
    };
  }, [open]);

  const displayName = user?.name || user?.login || authStrings.fallbackName;
  const detail = user?.detail || (user?.login ? `@${user.login}` : authStrings.fallbackDetail);
  const run = (action: () => void | Promise<void>) => () => {
    setOpen(false);
    void action();
  };

  return (
    <div ref={ref} className={`biotron-user-menu ${className}`.trim()}>
      <button
        ref={trigger}
        type="button"
        className="biotron-user-menu__trigger"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        aria-label={authStrings.userMenu}
      >
        {user?.avatarUrl ? <img src={user.avatarUrl} alt="" /> : <UserIcon />}
      </button>
      {open && (
        <div id={panelId} className="biotron-user-menu__popover">
          <div className="biotron-user-menu__identity">
            {/* Both lines are cut with an ellipsis in a fixed-width popover, so the full text goes in a tooltip. */}
            <strong title={displayName}>{displayName}</strong>
            <span title={detail}>{detail}</span>
          </div>
          <button type="button" className="biotron-user-menu__logout" onClick={run(onLogout)}>
            {logoutLabel}
          </button>
          {onLogoutEverywhere && (
            <button
              type="button"
              className="biotron-user-menu__logout"
              onClick={run(onLogoutEverywhere)}
            >
              {authStrings.signOutEverywhere}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
