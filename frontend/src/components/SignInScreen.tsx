import { AuthScreen } from '@biotron/style';
import { loginURL } from '../api';
import type { AuthNotice } from '../hooks/useAuthNotice';

const messages: Record<Exclude<AuthNotice, null>, string> = {
  denied: "Access denied — your GitHub account isn't a BioTronDesignTeam member.",
  banned: 'Your account has been banned. Contact an administrator if you believe this is a mistake.',
};

export default function SignInScreen({ notice }: { notice: AuthNotice }) {
  return (
    <AuthScreen
      productName="Auth"
      action={{ label: 'Sign in with GitHub', href: loginURL, icon: 'github' }}
      notices={notice ? [{ content: messages[notice], tone: 'error' as const }] : []}
    />
  );
}
