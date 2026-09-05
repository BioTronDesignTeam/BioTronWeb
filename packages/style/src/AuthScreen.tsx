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
  /** A named icon the screen draws itself, or any custom node. */
  icon?: 'github' | Exclude<ReactNode, string>;
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

/**
 * Notices carry no id of their own, so derive a stable one from what the notice
 * actually says. Two notices that read the same are disambiguated by an
 * occurrence suffix, which keeps keys unique without depending on array order.
 */
function noticeKeys(notices: readonly AuthNotice[]): string[] {
  const seen = new Map<string, number>();
  return notices.map((notice) => {
    const text = typeof notice.content === 'string' || typeof notice.content === 'number'
      ? String(notice.content)
      : 'node';
    const base = `${notice.tone ?? 'info'}:${text}`;
    const occurrence = seen.get(base) ?? 0;
    seen.set(base, occurrence + 1);
    return occurrence === 0 ? base : `${base}~${occurrence}`;
  });
}

function AuthNotices({ notices }: { notices: readonly AuthNotice[] }) {
  const keys = noticeKeys(notices);
  return (
    <>
      {notices.map((notice, position) => (
        <div
          className={`biotron-auth-notice biotron-auth-notice--${notice.tone ?? 'info'}`}
          role={notice.tone === 'error' ? 'alert' : 'status'}
          key={keys[position]}
        >
          {notice.content}
        </div>
      ))}
    </>
  );
}

function AuthActionControl({ action }: { action: AuthAction }) {
  const contents = (
    <>
      {action.icon === 'github' ? <GitHubMark /> : action.icon}
      {action.label}
    </>
  );

  if (action.href) {
    return <a className="biotron-auth-action" href={action.href}>{contents}</a>;
  }
  return (
    <button className="biotron-auth-action" type="button" onClick={action.onClick}>{contents}</button>
  );
}

function guestLabels(guestAccess: GuestAccess) {
  return {
    label: guestAccess.label ?? 'Daily guest key',
    placeholder: guestAccess.placeholder ?? 'XXXX-XXXX-XXXX',
    submitLabel: guestAccess.submitLabel ?? 'Continue as guest',
    submittingLabel: guestAccess.submittingLabel ?? 'Checking…',
    errorMessage: guestAccess.errorMessage ?? 'Invalid or expired key.',
  };
}

function useGuestSubmission(guestAccess: GuestAccess) {
  const [guestKey, setGuestKey] = useState('');
  const [failed, setFailed] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const change = (value: string) => {
    setGuestKey(value);
    setFailed(false);
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!guestKey.trim()) return;
    setSubmitting(true);
    setFailed(false);
    try {
      const accepted = await guestAccess.onSubmit(guestKey.trim());
      if (accepted === false) setFailed(true);
    } catch {
      setFailed(true);
    } finally {
      setSubmitting(false);
    }
  };

  return { guestKey, failed, submitting, change, submit };
}

function GuestAccessForm({ guestAccess }: { guestAccess: GuestAccess }) {
  const labels = guestLabels(guestAccess);
  const { guestKey, failed, submitting, change, submit } = useGuestSubmission(guestAccess);

  return (
    <form className="biotron-guest-form" onSubmit={submit}>
      <label htmlFor="biotron-guest-key">{labels.label}</label>
      <input
        id="biotron-guest-key"
        value={guestKey}
        onChange={(event) => change(event.target.value)}
        placeholder={labels.placeholder}
        autoComplete="off"
      />
      {failed && (
        <p className="biotron-guest-form__error" role="alert">
          {labels.errorMessage}
        </p>
      )}
      <button type="submit" disabled={submitting || !guestKey.trim()}>
        {submitting ? labels.submittingLabel : labels.submitLabel}
      </button>
    </form>
  );
}

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
  return (
    <main className={`biotron-auth-screen ${className}`.trim()}>
      {showThemeToggle && <ThemeToggle className="biotron-auth-screen__theme" />}
      <section className="biotron-auth-card">
        <Brand className="biotron-auth-card__brand" />
        <h1>{productName}</h1>
        {description && <div className="biotron-auth-card__description">{description}</div>}
        <AuthNotices notices={notices} />
        {action && <AuthActionControl action={action} />}
        {loading && <div className="biotron-auth-loading" aria-label="Loading" />}
        {guestAccess && (
          <>
            <div className="biotron-auth-divider"><span />or<span /></div>
            <GuestAccessForm guestAccess={guestAccess} />
          </>
        )}
        {footer && <div className="biotron-auth-card__footer">{footer}</div>}
      </section>
    </main>
  );
}
