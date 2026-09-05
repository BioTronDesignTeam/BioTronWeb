import { useEffect, useRef, useState } from 'react';

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
  className?: string;
};

export function UserMenu({
  user,
  onLogout,
  logoutLabel = 'Sign out',
  className = '',
}: UserMenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  const displayName = user?.name || user?.login || 'BioTron member';
  const detail = user?.detail || (user?.login ? `@${user.login}` : 'Authenticated account');

  return (
    <div ref={ref} className={`biotron-user-menu ${className}`.trim()}>
      <button
        type="button"
        className="biotron-user-menu__trigger"
        onClick={() => setOpen((value) => !value)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="User menu"
      >
        {user?.avatarUrl ? <img src={user.avatarUrl} alt="" /> : <UserIcon />}
      </button>
      {open && (
        <div className="biotron-user-menu__popover" role="menu">
          <div className="biotron-user-menu__identity">
            <strong>{displayName}</strong>
            <span>{detail}</span>
          </div>
          <button
            type="button"
            role="menuitem"
            className="biotron-user-menu__logout"
            onClick={() => {
              setOpen(false);
              void onLogout();
            }}
          >
            {logoutLabel}
          </button>
        </div>
      )}
    </div>
  );
}
