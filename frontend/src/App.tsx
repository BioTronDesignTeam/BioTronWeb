import { FormEvent, useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  APIError,
  type ApplicationStatus,
  type HealthState,
  type LogEntry,
  type LogLevel,
  accessManagerURL,
  checkSession,
  getApplications,
  getHistoricalLogs,
  getRecentLogs,
  loginURL,
} from './api';

const levels: LogLevel[] = ['debug', 'info', 'warning', 'error'];

type AccessState = 'checking' | 'allowed' | 'login' | 'forbidden' | 'error';
type LogMode = 'recent' | 'history';

function relativeTime(timestamp?: string) {
  if (!timestamp) return 'Not checked yet';
  const seconds = Math.round((new Date(timestamp).getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
  if (Math.abs(seconds) < 60) return formatter.format(seconds, 'second');
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, 'hour');
  return formatter.format(Math.round(hours / 24), 'day');
}

function stateLabel(state: HealthState) {
  if (state === 'healthy') return 'Operational';
  if (state === 'unhealthy') return 'Unavailable';
  return 'Awaiting check';
}

function StatusDot({ state }: { state: HealthState }) {
  return <span className={`status-dot status-dot--${state}`} aria-hidden="true" />;
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="shell">
      <header className="topbar">
        <a className="brand" href="/">
          <span className="brand-mark">B</span>
          <span>
            <strong>BioTron</strong>
            <small>Operations</small>
          </span>
        </a>
        <div className="environment"><span /> Platform monitor</div>
      </header>
      {children}
    </div>
  );
}

function AccessGate({ state }: { state: AccessState }) {
  const content = {
    checking: ['Checking access', 'Confirming your BioTron session…'],
    login: ['Sign in required', 'Use your BioTron GitHub account to view internal service logs.'],
    forbidden: ['Logger access required', 'You are signed in, but do not have the Logger read permission.'],
    error: ['Logger is unavailable', 'The portal could not verify your session. Try again shortly.'],
    allowed: ['', ''],
  }[state];

  return (
    <Shell>
      <main className="access-page">
        <div className="access-panel">
          <p className="eyebrow">Internal tooling</p>
          <h1>{content[0]}</h1>
          <p>{content[1]}</p>
          {state === 'login' && <a className="button button--primary" href={loginURL()}>Continue with GitHub</a>}
          {state === 'forbidden' && <a className="button button--primary" href={accessManagerURL()}>Request access</a>}
          {state === 'error' && <button className="button button--primary" onClick={() => window.location.reload()}>Retry</button>}
          {state === 'checking' && <div className="loading-bar" />}
        </div>
      </main>
    </Shell>
  );
}

function Dashboard({ applications, refreshedAt }: { applications: ApplicationStatus[]; refreshedAt?: string }) {
  const healthy = applications.filter((app) => app.state === 'healthy').length;
  const headline = applications.some((app) => app.state === 'unhealthy')
    ? 'Some systems need attention'
    : applications.some((app) => app.state === 'unknown')
      ? 'Health checks are warming up'
      : 'All systems operational';

  return (
    <Shell>
      <main className="page">
        <section className="hero">
          <div>
            <p className="eyebrow">Live platform status</p>
            <h1>{headline}</h1>
            <p className="hero-copy">Health is checked from inside the BioTron service network.</p>
          </div>
          <div className="summary">
            <strong>{healthy}/{applications.length}</strong>
            <span>applications operational</span>
            <small>Updated {relativeTime(refreshedAt)}</small>
          </div>
        </section>

        <section className="section-heading">
          <div>
            <h2>Applications</h2>
            <p>Select an application to inspect components and logs.</p>
          </div>
        </section>

        <div className="application-grid">
          {applications.map((app) => (
            <a className="application-card" href={`/applications/${app.id}`} key={app.id}>
              <div className="application-card__top">
                <div>
                  <h3>{app.name}</h3>
                  <p>{app.description}</p>
                </div>
                <span className={`state-pill state-pill--${app.state}`}>
                  <StatusDot state={app.state} />{stateLabel(app.state)}
                </span>
              </div>
              <div className="component-list">
                {app.components.map((component) => (
                  <div className="component-row" key={component.id}>
                    <span><StatusDot state={component.state} />{component.name}</span>
                    <small>{relativeTime(component.checked_at)}</small>
                  </div>
                ))}
              </div>
              <span className="inspect-link">Inspect application <span aria-hidden="true">→</span></span>
            </a>
          ))}
        </div>
      </main>
    </Shell>
  );
}

function LevelFilter({ selected, onChange }: { selected: Set<LogLevel>; onChange: (next: Set<LogLevel>) => void }) {
  return (
    <fieldset className="level-filter">
      <legend>Log levels</legend>
      {levels.map((level) => (
        <label className={`level-toggle level-toggle--${level}`} key={level}>
          <input
            checked={selected.has(level)}
            type="checkbox"
            onChange={() => {
              const next = new Set(selected);
              if (next.has(level)) next.delete(level); else next.add(level);
              onChange(next);
            }}
          />
          <span>{level}</span>
        </label>
      ))}
    </fieldset>
  );
}

function LogRow({ entry }: { entry: LogEntry }) {
  const hasPayload = entry.payload !== undefined && entry.payload !== null;
  return (
    <article className="log-row">
      <div className="log-row__meta">
        <span className={`log-level log-level--${entry.level}`}>{entry.level}</span>
        <span className="service-name">{entry.service}</span>
        <time dateTime={entry.created_at} title={new Date(entry.created_at).toLocaleString()}>
          {relativeTime(entry.created_at)}
        </time>
      </div>
      <p>{entry.message}</p>
      {hasPayload && (
        <details>
          <summary>Structured payload</summary>
          <pre>{JSON.stringify(entry.payload, null, 2)}</pre>
        </details>
      )}
    </article>
  );
}

function ApplicationDetail({ application }: { application: ApplicationStatus }) {
  const [mode, setMode] = useState<LogMode>('recent');
  const [selectedLevels, setSelectedLevels] = useState<Set<LogLevel>>(new Set(levels));
  const [search, setSearch] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const filters = useMemo(() => ({
    levels: Array.from(selectedLevels),
    search,
    from: from ? new Date(from).toISOString() : undefined,
    to: to ? new Date(to).toISOString() : undefined,
    limit: 100,
  }), [from, search, selectedLevels, to]);
  const [appliedFilters, setAppliedFilters] = useState(filters);

  const load = useCallback(async (append = false, cursor?: string) => {
    setLoading(true);
    setError('');
    try {
      const page = mode === 'recent'
        ? await getRecentLogs(application.id, appliedFilters)
        : await getHistoricalLogs(application.id, { ...appliedFilters, cursor });
      setLogs((current) => append ? [...current, ...page.logs] : page.logs);
      setNextCursor(page.next_cursor);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Unable to load logs');
    } finally {
      setLoading(false);
    }
  }, [application.id, appliedFilters, mode]);

  useEffect(() => {
    void load();
    if (mode !== 'recent') return;
    const timer = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(timer);
  }, [load, mode]);

  function applyFilters(event: FormEvent) {
    event.preventDefault();
    setAppliedFilters(filters);
  }

  return (
    <Shell>
      <main className="page detail-page">
        <a className="back-link" href="/">← All applications</a>
        <section className="detail-header">
          <div>
            <div className="detail-title">
              <h1>{application.name}</h1>
              <span className={`state-pill state-pill--${application.state}`}>
                <StatusDot state={application.state} />{stateLabel(application.state)}
              </span>
            </div>
            <p>{application.description}</p>
          </div>
        </section>

        <section className="component-strip" aria-label="Component health">
          {application.components.map((component) => (
            <div className="component-tile" key={component.id}>
              <div><StatusDot state={component.state} /><strong>{component.name}</strong></div>
              <span>{component.detail || stateLabel(component.state)}</span>
              <small>{relativeTime(component.checked_at)}</small>
            </div>
          ))}
        </section>

        <section className="logs-panel">
          <div className="logs-panel__heading">
            <div>
              <p className="eyebrow">Application events</p>
              <h2>Logs</h2>
            </div>
            <div className="mode-switch" role="group" aria-label="Log source">
              <button className={mode === 'recent' ? 'active' : ''} onClick={() => setMode('recent')}>Recent</button>
              <button className={mode === 'history' ? 'active' : ''} onClick={() => setMode('history')}>History</button>
            </div>
          </div>

          <form className="filters" onSubmit={applyFilters}>
            <LevelFilter selected={selectedLevels} onChange={setSelectedLevels} />
            <label className="search-field">
              <span>Search messages and payloads</span>
              <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search logs…" />
            </label>
            {mode === 'history' && (
              <div className="date-fields">
                <label><span>From</span><input type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} /></label>
                <label><span>To</span><input type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} /></label>
              </div>
            )}
            <button className="button button--secondary" type="submit">Apply filters</button>
          </form>

          <div className="log-stream" aria-live="polite">
            {error && <div className="inline-error">{error}</div>}
            {!loading && !error && logs.length === 0 && (
              <div className="empty-state"><strong>No matching logs</strong><span>Try another level, time range, or search.</span></div>
            )}
            {logs.map((entry) => <LogRow entry={entry} key={entry.id} />)}
            {loading && <div className="stream-loading">Loading logs…</div>}
          </div>
          {mode === 'history' && nextCursor && !loading && (
            <button className="button button--secondary load-more" onClick={() => void load(true, nextCursor)}>Load older logs</button>
          )}
        </section>
      </main>
    </Shell>
  );
}

export function App() {
  const [access, setAccess] = useState<AccessState>('checking');
  const [applications, setApplications] = useState<ApplicationStatus[]>([]);
  const [refreshedAt, setRefreshedAt] = useState<string>();

  const refresh = useCallback(async () => {
    try {
      const response = await getApplications();
      setApplications(response.applications);
      setRefreshedAt(response.generated_at);
      setAccess('allowed');
    } catch (reason) {
      if (reason instanceof APIError && reason.status === 401) setAccess('login');
      else if (reason instanceof APIError && reason.status === 403) setAccess('forbidden');
      else setAccess('error');
    }
  }, []);

  useEffect(() => {
    void checkSession().then(refresh).catch((reason: unknown) => {
      if (reason instanceof APIError && reason.status === 401) setAccess('login');
      else if (reason instanceof APIError && reason.status === 403) setAccess('forbidden');
      else setAccess('error');
    });
  }, [refresh]);

  useEffect(() => {
    if (access !== 'allowed') return;
    const timer = window.setInterval(() => void refresh(), 15000);
    return () => window.clearInterval(timer);
  }, [access, refresh]);

  if (access !== 'allowed') return <AccessGate state={access} />;

  const detailMatch = window.location.pathname.match(/^\/applications\/([^/]+)\/?$/);
  if (detailMatch) {
    const application = applications.find((candidate) => candidate.id === decodeURIComponent(detailMatch[1]));
    if (application) return <ApplicationDetail application={application} />;
    return <AccessGate state="error" />;
  }

  return <Dashboard applications={applications} refreshedAt={refreshedAt} />;
}
