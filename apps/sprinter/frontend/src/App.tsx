import { useCallback, useEffect, useState } from 'react';
import { AuthScreen } from '@biotron/style';
import { login, sprinterApi } from './api';
import type { Automation, AutomationPayload, AuthStatus, Guard, GuardSubject, Scope } from './types';
import { AutomationsPanel } from './components/AutomationsPanel';
import { GuardsPanel } from './components/GuardsPanel';
import { Header } from './components/Header';

export function App() {
  const [auth, setAuth] = useState<AuthStatus>({ can_admin: false });
  const [authLoaded, setAuthLoaded] = useState(false);
  const [guards, setGuards] = useState<Guard[]>([]);
  const [automations, setAutomations] = useState<Automation[]>([]);
  const [scopes, setScopes] = useState<Scope[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadAuth = useCallback(async () => {
    try {
      setAuth(await sprinterApi.authStatus());
    } catch {
      setAuth({ can_admin: false });
    } finally {
      setAuthLoaded(true);
    }
  }, []);

  useEffect(() => {
    void loadAuth();
  }, [loadAuth]);

  const loadAdmin = useCallback(async () => {
    if (!auth.can_admin) return;
    setLoading(true);
    setError('');
    try {
      const [nextGuards, nextAutomations] = await Promise.all([sprinterApi.guards(), sprinterApi.automations()]);
      setGuards(nextGuards);
      setAutomations(nextAutomations);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not load Sprinter administration.');
    }
    // The Calendar scopes list lives on a separate service, so its own
    // failure should not blank out guards and automations that already loaded.
    try {
      setScopes(await sprinterApi.scopes());
    } catch (caught) {
      setError((current) => current || (caught instanceof Error ? caught.message : 'Could not load calendar scopes.'));
    }
    setLoading(false);
  }, [auth.can_admin]);

  useEffect(() => {
    void loadAdmin();
  }, [loadAdmin]);

  async function saveGuard(subject: GuardSubject, body: { guild_id: string; role_ids: string[]; channel_ids: string[] }) {
    const updated = await sprinterApi.saveGuard(subject, body);
    setGuards((current) => [...current.filter((guard) => guard.subject !== subject), updated]);
  }

  async function clearGuard(subject: GuardSubject) {
    await sprinterApi.clearGuard(subject);
    setGuards((current) => current.filter((guard) => guard.subject !== subject));
  }

  async function createAutomation(body: AutomationPayload) {
    const created = await sprinterApi.createAutomation(body);
    setAutomations((current) => [...current, created]);
  }

  async function updateAutomation(id: string, body: Partial<AutomationPayload>) {
    const updated = await sprinterApi.updateAutomation(id, body);
    setAutomations((current) => current.map((automation) => (automation.id === id ? updated : automation)));
  }

  async function deleteAutomation(automation: Automation) {
    await sprinterApi.deleteAutomation(automation.id);
    setAutomations((current) => current.filter((candidate) => candidate.id !== automation.id));
  }

  if (!authLoaded) {
    return <div className="min-h-dvh bg-white dark:bg-page" />;
  }

  if (!auth.operator) {
    return (
      <AuthScreen
        productName="Sprinter"
        description="Sign in with GitHub to manage the BioTron Discord bot."
        action={{ label: 'Log in with GitHub', icon: 'github', onClick: login }}
      />
    );
  }

  return (
    <div className="min-h-dvh bg-white text-ink dark:bg-page dark:text-white">
      <Header auth={auth} onLoggedOut={() => setAuth({ can_admin: false })} />
      <main className="mx-auto w-full max-w-[1200px] space-y-8 px-4 pb-16 pt-8 sm:px-6 lg:px-8">
        {!auth.can_admin ? (
          <p className="rounded-2xl border border-ink/10 bg-white px-5 py-4 text-sm dark:border-line dark:bg-surface">
            Sprinter administration needs the sprinter/admin permission. Ask a manager in Auth to grant it.
          </p>
        ) : (
          <>
            {error && <p className="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{error}</p>}
            {loading && <div className="h-1 animate-pulse rounded-full bg-brand" />}
            <GuardsPanel guards={guards} onSave={saveGuard} onClear={clearGuard} />
            <AutomationsPanel
              automations={automations}
              scopes={scopes}
              onCreate={createAutomation}
              onUpdate={updateAutomation}
              onDelete={deleteAutomation}
            />
          </>
        )}
      </main>
    </div>
  );
}
