/**
 * Every user-facing word about signing in, roles, and permissions, in one
 * place. The glossary behind it is in AGENTS.md ("Wording"): "Sign in" and
 * "Sign out", never "log in"; the roles Superuser, Manager, Member, and Guest;
 * "permission" for the thing, "grant" and "revoke" for the actions; the
 * "Daily guest key"; and "Ask a manager." as the one line about who to ask.
 */
export const authStrings = {
  signIn: 'Sign in',
  signInWithGitHub: 'Sign in with GitHub',
  signOut: 'Sign out',
  signOutFailed: 'Sign out failed. Try again.',
  lastSignIn: 'Last sign-in',
  loading: 'Loading…',
  askAManager: 'Ask a manager.',

  denied: "Access denied. Your GitHub account isn't a BioTronDesignTeam member.",
  banned: 'Your account has been banned. Ask a manager.',
  expired: 'Your session has expired. Sign in again.',

  accessDenied: 'Access denied',
  noPermission: (product: string) => `You don't have permission to use ${product}. Ask a manager.`,
  signedInAs: (login: string) => `Signed in as @${login}`,

  unavailable: "Can't reach BioTron sign-in right now. Retrying…",
  unavailableShort: 'Sign-in unavailable',
  retry: 'Try again',

  forbidden: "You don't have permission to do that. Ask a manager.",
  serviceUnavailable: "Can't reach BioTron sign-in right now. Try again in a moment.",

  guestKeyLabel: 'Daily guest key',
  guestKeyPlaceholder: 'XXXX-XXXX-XXXX',
  guestSubmit: 'Continue as guest',
  guestSubmitting: 'Checking…',
  guestInvalid: 'Invalid or expired key.',

  fallbackName: 'BioTron member',
  fallbackDetail: 'Signed in',
  userMenu: 'User menu',
  dismiss: 'Dismiss',
} as const;

export type Role = 'Superuser' | 'Manager' | 'Member' | 'Guest';

type RoleFlags = {
  is_superuser?: boolean;
  is_manager?: boolean;
  is_guest?: boolean;
};

/** The one role a person holds, highest first. "Staff" is a code word only. */
export function roleOf(person: RoleFlags): Role {
  if (person.is_guest) return 'Guest';
  if (person.is_superuser) return 'Superuser';
  if (person.is_manager) return 'Manager';
  return 'Member';
}

/** `@login · Role`, the identity line under a name. */
export function identityLine(person: RoleFlags & { login: string }): string {
  return `@${person.login} · ${roleOf(person)}`;
}

/**
 * Backends answer refusals with terse status text ("unauthorized",
 * "Forbidden", "service unavailable", "HTTP 403"). None of that is meant for a
 * person. This turns a failed response into a shared sentence and keeps any
 * other, specific message the server sent.
 */
export function friendlyError(status: number, message?: string): string {
  const raw = (message ?? '').trim();
  const lowered = raw.toLowerCase();
  if (status === 401 || lowered === 'unauthorized' || lowered === 'authentication required') {
    return authStrings.expired;
  }
  if (
    lowered === 'forbidden'
    || lowered === 'permission required'
    || (status === 403 && (raw === '' || /^http 403$/i.test(raw)))
  ) {
    return authStrings.forbidden;
  }
  if (
    status === 503
    || lowered === 'service unavailable'
    || lowered === 'authorization service unavailable'
  ) {
    return authStrings.serviceUnavailable;
  }
  if (raw === '' || /^http \d{3}$/i.test(raw) || /^request failed \(\d{3}\)$/i.test(raw)) {
    return `Something went wrong (${status}). Try again.`;
  }
  return raw;
}

/** Whether a failed response means the session is gone. */
export function isSessionExpired(status: number): boolean {
  return status === 401;
}

/** The sentence for a sign-in notice (`?auth=denied|banned`, or an expired session). */
export function noticeMessage(notice: 'denied' | 'banned' | 'expired'): string {
  return authStrings[notice];
}
