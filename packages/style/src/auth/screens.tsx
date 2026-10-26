import type { ReactNode } from 'react';
import { AuthScreen, type AuthNotice } from '../AuthScreen';
import { useSession, type SessionNotice } from './session';
import { authStrings, noticeMessage } from './strings';

function noticeList(notice: SessionNotice | null): AuthNotice[] {
  if (!notice) return [];
  return [{ content: noticeMessage(notice), tone: notice === 'expired' ? 'info' : 'error' }];
}

export type SignInScreenProps = {
  productName: string;
  description?: ReactNode;
  /** Offer the daily guest key form. Needs `appId` on the provider. */
  guest?: boolean;
};

/** The sign-in card, with the denied, banned, or expired notice when there is one. */
export function SignInScreen({ productName, description, guest = false }: SignInScreenProps) {
  const { login, loginGuest, notice } = useSession();
  return (
    <AuthScreen
      productName={productName}
      description={description}
      action={{ label: authStrings.signInWithGitHub, onClick: login, icon: 'github' }}
      notices={noticeList(notice)}
      guestAccess={guest ? { onSubmit: loginGuest } : undefined}
    />
  );
}

export type AccessDeniedProps = {
  productName: string;
  /** Replaces the default "You don't have permission to use …" line. */
  message?: ReactNode;
};

/** For a signed-in person without the permission the page needs. */
export function AccessDenied({ productName, message }: AccessDeniedProps) {
  const { operator, logout } = useSession();
  return (
    <AuthScreen
      productName={productName}
      notices={[
        { content: authStrings.accessDenied, tone: 'error' },
        { content: message ?? authStrings.noPermission(productName) },
      ]}
      action={{ label: authStrings.signOut, onClick: () => void logout().catch(() => undefined) }}
      footer={operator ? authStrings.signedInAs(operator.login) : undefined}
    />
  );
}

/** Shown while Auth cannot be reached and nobody was signed in yet. */
export function SessionUnavailable({ productName }: { productName: string }) {
  const { refresh } = useSession();
  return (
    <AuthScreen
      productName={productName}
      notices={[{ content: authStrings.unavailable }]}
      action={{ label: authStrings.retry, onClick: () => void refresh() }}
      loading
    />
  );
}

/** The full-page placeholder while the first session check runs. */
export function SessionLoading() {
  return (
    <output className="biotron-session-loading">
      {authStrings.loading}
    </output>
  );
}

export type RequireAuthProps = {
  productName: string;
  /** Needed to see the page: one key, or a list where any one is enough. */
  permission?: string | readonly string[];
  description?: ReactNode;
  guest?: boolean;
  children: ReactNode;
};

/**
 * Gates a page on a session and, optionally, a permission: loading, then
 * sign-in, the outage screen, or Access denied, and the page only once the
 * session holds what it needs.
 */
export function RequireAuth({ productName, permission, description, guest, children }: RequireAuthProps) {
  const { status, can } = useSession();
  if (status === 'loading') return <SessionLoading />;
  if (status === 'unavailable') return <SessionUnavailable productName={productName} />;
  if (status === 'anon') return <SignInScreen productName={productName} description={description} guest={guest} />;
  if (permission && !can(permission)) return <AccessDenied productName={productName} />;
  return <>{children}</>;
}
