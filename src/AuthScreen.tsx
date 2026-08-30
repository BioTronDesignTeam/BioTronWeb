import { useState, type FormEvent, type ReactNode } from 'react';
import { Brand } from './Brand';
import { ThemeToggle } from './ThemeToggle';

const GitHubMark = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M12 .5C5.73.5.5 5.73.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56 0-.28-.01-1.02-.02-2-3.2.7-3.88-1.54-3.88-1.54-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.23-1.28-5.23-5.7 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.1 11.1 0 0 1 5.8 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.12 3.05.74.81 1.18 1.84 1.18 3.1 0 4.43-2.69 5.41-5.25 5.69.41.36.78 1.06.78 2.14 0 1.55-.01 2.8-.01 3.18 0 .31.21.68.8.56A11.51 11.51 0 0 0 23.5 12C23.5 5.73 18.27.5 12 .5Z" />
  </svg>
);

export type AuthNotice = {
  content: ReactNode;
  tone?: 'info' | 'error';
};

export type AuthAction = {
  label: string;
  href?: string;
  onClick?: () => void;
  icon?: 'github' | ReactNode;
};

export type GuestAccess = {
  onSubmit: (key: string) => boolean | void | Promise<boolean | void>;
  label?: string;
  placeholder?: string;
  submitLabel?: string;
  submittingLabel?: string;
  errorMessage?: string;
};

export type AuthScreenProps = {
  productName: string;
  description?: ReactNode;
  action?: AuthAction;
  notices?: readonly AuthNotice[];
  guestAccess?: GuestAccess;
  loading?: boolean;
  footer?: ReactNode;
  showThemeToggle?: boolean;
  className?: string;
};

export function AuthScreen({
  productName,
  description,
  action,
  notices = [],
  guestAccess,
  loading = false,
  footer,
  showThemeToggle = true,
  className = '',
}: AuthScreenProps) {
  const [guestKey, setGuestKey] = useState('');
  const [guestError, setGuestError] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function submitGuest(event: FormEvent) {
    event.preventDefault();
    if (!guestAccess || !guestKey.trim()) return;
    setSubmitting(true);
    setGuestError(false);
    try {
      const accepted = await guestAccess.onSubmit(guestKey.trim());
      if (accepted === false) setGuestError(true);
    } catch {
      setGuestError(true);
    } finally {
      setSubmitting(false);
    }
  }

  const actionContents = (
    <>
      {action?.icon === 'github' ? <GitHubMark /> : action?.icon}
      {action?.label}
    </>
  );

  return (
    <main className={`biotron-auth-screen ${className}`.trim()}>
      {showThemeToggle && <ThemeToggle className="biotron-auth-screen__theme" />}
      <section className="biotron-auth-card">
        <Brand className="biotron-auth-card__brand" />
        <h1>{productName}</h1>
        {description && <div className="biotron-auth-card__description">{description}</div>}
        {notices.map((notice, index) => (
          <div
            className={`biotron-auth-notice biotron-auth-notice--${notice.tone ?? 'info'}`}
            role={notice.tone === 'error' ? 'alert' : 'status'}
            key={index}
          >
            {notice.content}
          </div>
        ))}
        {action && (action.href ? (
          <a className="biotron-auth-action" href={action.href}>{actionContents}</a>
        ) : (
          <button className="biotron-auth-action" type="button" onClick={action.onClick}>{actionContents}</button>
        ))}
        {loading && <div className="biotron-auth-loading" aria-label="Loading" />}
        {guestAccess && (
          <>
            <div className="biotron-auth-divider"><span />or<span /></div>
            <form className="biotron-guest-form" onSubmit={submitGuest}>
              <label htmlFor="biotron-guest-key">{guestAccess.label ?? 'Daily guest key'}</label>
              <input
                id="biotron-guest-key"
                value={guestKey}
                onChange={(event) => {
                  setGuestKey(event.target.value);
                  setGuestError(false);
                }}
                placeholder={guestAccess.placeholder ?? 'XXXX-XXXX-XXXX'}
                autoComplete="off"
              />
              {guestError && (
                <p className="biotron-guest-form__error" role="alert">
                  {guestAccess.errorMessage ?? 'Invalid or expired key.'}
                </p>
              )}
              <button type="submit" disabled={submitting || !guestKey.trim()}>
                {submitting
                  ? guestAccess.submittingLabel ?? 'Checking…'
                  : guestAccess.submitLabel ?? 'Continue as guest'}
              </button>
            </form>
          </>
        )}
        {footer && <div className="biotron-auth-card__footer">{footer}</div>}
      </section>
    </main>
  );
}
