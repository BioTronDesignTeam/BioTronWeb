import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import { Brand, Button, ThemeToggle, UserMenu } from '@biotron/style';
import {
  type ApplicationStatus,
  type ComponentHistory,
  type HealthState,
  type HistoryBucket,
  type Identity,
  type LogEntry,
  type LogLevel,
  type Session,
  type StatusApplication,
  type StatusResponse,
  type StatusState,
  accessManagerURL,
  checkSession,
  getIdentity,
  getApplications,
  getHistoricalLogs,
  getRecentLogs,
  getStatus,
  getStatusHistory,
  loginURL,
  logout,
} from './api';

const levels: LogLevel[] = ['debug', 'info', 'warning', 'error'];
const HISTORY_DAYS = 90;

// Building an Intl formatter is expensive, so the shared instances live here
// rather than being rebuilt inside a function that runs on every render.
const relativeFormatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
const dateFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric' });

type LogMode = 'recent' | 'history';

/**
 * Status is never conveyed by colour alone: every state carries a glyph and a
 * word as well, so the page still reads correctly in monochrome or to anyone
 * who cannot separate the hues.
 */
const statusMeta: Record<StatusState, { label: string; glyph: string; headline: string }> = {
  operational: { label: 'Operational', glyph: '✓', headline: 'All systems operational' },
  degraded: { label: 'Degraded', glyph: '!', headline: 'Some systems are degraded' },
  down: { label: 'Down', glyph: '✕', headline: 'Major outage' },
  unknown: { label: 'No data', glyph: '?', headline: 'Status is not being observed' },
};

const healthMeta: Record<HealthState, { label: string; glyph: string }> = {
  healthy: { label: 'Operational', glyph: '✓' },
  unhealthy: { label: 'Unavailable', glyph: '✕' },
  unknown: { label: 'Awaiting check', glyph: '?' },
};

function relativeTime(timestamp?: string | null) {
  if (!timestamp) return 'Not checked yet';
  const seconds = Math.round((new Date(timestamp).getTime() - Date.now()) / 1000);
  if (Math.abs(seconds) < 60) return relativeFormatter.format(seconds, 'second');
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return relativeFormatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return relativeFormatter.format(hours, 'hour');
  return relativeFormatter.format(Math.round(hours / 24), 'day');
}

/** A null uptime means the window was never observed, which is not 100%. */
function uptimeLabel(value: number | null | undefined) {
  return value === null || value === undefined ? 'No data' : `${value.toFixed(2)}%`;
}

function StatusBadge({ state, compact }: { state: StatusState; compact?: boolean }) {
  const meta = statusMeta[state];
  return (
    <span className={`status-badge status-badge--${state}`}>
      <span className="status-badge__glyph" aria-hidden="true">{meta.glyph}</span>
      {!compact && <span className="status-badge__label">{meta.label}</span>}
      {compact && <span className="visually-hidden">{meta.label}</span>}
    </span>
  );
}

function HealthBadge({ state }: { state: HealthState }) {
  const meta = healthMeta[state];
  return (
    <span className={`status-badge status-badge--${state}`}>
      <span className="status-badge__glyph" aria-hidden="true">{meta.glyph}</span>
      <span className="status-badge__label">{meta.label}</span>
    </span>
  );
}

/**
 * How many days of the bar fit without pushing the page sideways. A phone gets
 * thirty; the bar also scrolls inside its own container, so even an
 * unanticipated width can never make the page body scroll horizontally.
 */
const dayBreakpoints = [
  { query: '(max-width: 559px)', days: 30 },
  { query: '(max-width: 899px)', days: 60 },
];

function subscribeToWidth(onChange: () => void) {
  const lists = dayBreakpoints.map((breakpoint) => window.matchMedia(breakpoint.query));
  lists.forEach((list) => list.addEventListener('change', onChange));
  // resize is a belt-and-braces fallback for embedded viewports that resize
  // without firing media-query change events. The snapshot is a number, so a
  // resize that does not cross a breakpoint re-renders nothing.
  window.addEventListener('resize', onChange);
  return () => {
    lists.forEach((list) => list.removeEventListener('change', onChange));
    window.removeEventListener('resize', onChange);
  };
}

function currentVisibleDays() {
  const matched = dayBreakpoints.find((breakpoint) => window.matchMedia(breakpoint.query).matches);
  return matched ? matched.days : HISTORY_DAYS;
}

const useVisibleDays = () =>
  useSyncExternalStore(subscribeToWidth, currentVisibleDays, () => HISTORY_DAYS);

function bucketTitle(bucket: HistoryBucket) {
  const date = dateFormatter.format(new Date(`${bucket.date}T12:00:00`));
  return `${date} — ${statusMeta[bucket.state].label}, ${uptimeLabel(bucket.uptime)}`;
}

function UptimeBar({ buckets, days }: { buckets: HistoryBucket[]; days: number }) {
  const shown = buckets.slice(-days);
  const summary = `${days}-day history: ${shown.filter((bucket) => bucket.state === 'operational').length} fully operational days, ` +
    `${shown.filter((bucket) => bucket.state === 'unknown').length} without data`;

  return (
    <div className="uptime-bar">
      <div className="uptime-bar__scroll">
        <div className="uptime-bar__track" role="img" aria-label={summary}>
          {shown.map((bucket) => (
            <span
              className={`uptime-day uptime-day--${bucket.state}`}
              key={bucket.date}
              title={bucketTitle(bucket)}
            />
          ))}
        </div>
      </div>
      <div className="uptime-bar__legend">
        <span>{days} days ago</span>
        <span aria-hidden="true" className="uptime-bar__rule" />
        <span>Today</span>
      </div>
    </div>
  );
}

function UptimeFigures({
  values,
}: {
  values: { uptime_24h: number | null; uptime_7d: number | null; uptime_90d: number | null };
}) {
  return (
    <dl className="uptime-figures">
      <div><dt>24 hours</dt><dd>{uptimeLabel(values.uptime_24h)}</dd></div>
      <div><dt>7 days</dt><dd>{uptimeLabel(values.uptime_7d)}</dd></div>
      <div><dt>90 days</dt><dd>{uptimeLabel(values.uptime_90d)}</dd></div>
    </dl>
  );
}

function Shell({
  children,
  session,
  user,
  onLogout,
}: {
  children: ReactNode;
  session: Session;
  user?: Identity;
  onLogout: () => void | Promise<void>;
}) {
  return (
    <div className="shell">
      <header className="topbar">
        <a className="brand" href="/">
          <Brand compact />
          <span>
            <strong>Logger</strong>
            <small>Platform status</small>
          </span>
        </a>
        <div className="topbar-actions">
          <ThemeToggle />
          {session.authenticated ? (
            <UserMenu
              user={user ? {
                name: user.name,
                login: user.login,
                avatarUrl: user.avatar_url,
                detail: `@${user.login}${user.is_guest ? ' · guest' : user.is_staff ? ' · staff' : ''}`,
              } : undefined}
              onLogout={onLogout}
            />
          ) : (
            <Button tone="primary" onClick={() => { window.location.href = loginURL(); }}>
              Sign in
            </Button>
          )}
        </div>
      </header>
      {children}
    </div>
  );
}

/**
 * Shown to a real session that lacks logger/view. It deliberately offers no
 * sign-in button: they are already signed in, and sending them back to the IdP
 * is the loop this page exists to end.
 */
function AccessNotice() {
  return (
    <section className="access-notice">
      <h2>You do not have Logger access</h2>
      <p>
        You are signed in, but your account does not hold the <code>logger</code> ·{' '}
        <code>view</code> permission, so the log explorer stays hidden. Everything on this status
        page is public and needs no permission at all.
      </p>
      <a className="button button--secondary" href={accessManagerURL()}>
        Request access in OAuth Manager
      </a>
    </section>
  );
}

function ApplicationRow({
  application,
  history,
  days,
  explorable,
}: {
  application: StatusApplication;
  history: Map<string, ComponentHistory>;
  days: number;
  explorable: boolean;
}) {
  return (
    <article className="application-row">
      <div className="application-row__head">
        <div>
          <h3>{application.name}</h3>
          <p>{application.description}</p>
        </div>
        <StatusBadge state={application.state} />
      </div>

      <div className="component-rows">
        {application.components.map((component) => {
          const buckets = history.get(component.id)?.buckets ?? [];
          return (
            <div className="component-row" key={component.id}>
              <div className="component-row__name">
                <StatusBadge state={component.state} compact />
                <strong>{component.name}</strong>
                <small>Checked {relativeTime(component.checked_at)}</small>
              </div>
              {buckets.length > 0 && <UptimeBar buckets={buckets} days={days} />}
              <UptimeFigures values={component} />
            </div>
          );
        })}
      </div>

      {explorable && (
        <a className="inspect-link" href={`/applications/${application.id}`}>
          Open logs <span aria-hidden="true">→</span>
        </a>
      )}
    </article>
  );
}

function StatusPage({
  session,
  user,
  onLogout,
}: {
  session: Session;
  user?: Identity;
  onLogout: () => void | Promise<void>;
}) {
  const [status, setStatus] = useState<StatusResponse>();
  const [history, setHistory] = useState<ComponentHistory[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const days = useVisibleDays();

  const refresh = useCallback(async () => {
    try {
      // Both requests are unauthenticated, so this runs identically for a
      // signed-out visitor and never depends on a session being resolved first.
      const [current, past] = await Promise.all([getStatus(), getStatusHistory(HISTORY_DAYS)]);
      setStatus(current);
      setHistory(past.components);
      setError('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Status is unavailable');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 30000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const historyByComponent = useMemo(() => {
    const index = new Map<string, ComponentHistory>();
    history.forEach((component) => index.set(component.id, component));
    return index;
  }, [history]);

  const overall = status?.overall;
  const meta = statusMeta[overall?.state ?? 'unknown'];

  return (
    <Shell session={session} user={user} onLogout={onLogout}>
      <main className="page">
        {session.authenticated && !session.allowed && <AccessNotice />}

        <section className={`banner banner--${overall?.state ?? 'unknown'}`}>
          <span className="banner__glyph" aria-hidden="true">{meta.glyph}</span>
          <div>
            <h1>{loading && !overall ? 'Checking platform status…' : meta.headline}</h1>
            <p>
              {overall
                ? `Updated ${relativeTime(overall.updated_at)} · checked from inside the BioTron service network`
                : 'Health is checked from inside the BioTron service network.'}
            </p>
          </div>
        </section>

        {error && <div className="inline-error">{error}</div>}

        {overall && (
          <section className="headline-uptime" aria-label="Platform uptime">
            <UptimeFigures values={overall} />
          </section>
        )}

        <section className="section-heading">
          <div>
            <h2>Applications</h2>
            <p>
              {session.allowed
                ? 'Select an application to inspect its logs.'
                : 'Component availability over the last 90 days.'}
            </p>
          </div>
        </section>

        <div className="application-list">
          {status?.applications.map((application) => (
            <ApplicationRow
              application={application}
              days={days}
              explorable={session.allowed}
              history={historyByComponent}
              key={application.id}
            />
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
  // Lazy initialiser: the Set is built once, not thrown away on every render.
  const [selectedLevels, setSelectedLevels] = useState<Set<LogLevel>>(() => new Set(levels));
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
    <main className="page detail-page">
      <a className="back-link" href="/">← Platform status</a>
      <section className="detail-header">
        <div>
          <div className="detail-title">
            <h1>{application.name}</h1>
            <HealthBadge state={application.state} />
          </div>
          <p>{application.description}</p>
        </div>
      </section>

      <section className="component-strip" aria-label="Component health">
        {application.components.map((component) => (
          <div className="component-tile" key={component.id}>
            <div><HealthBadge state={component.state} /><strong>{component.name}</strong></div>
            <span>{component.detail || healthMeta[component.state].label}</span>
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
  );
}

function ExplorerRoute({
  applicationID,
  session,
  sessionReady,
  user,
  onLogout,
}: {
  applicationID: string;
  session: Session;
  sessionReady: boolean;
  user?: Identity;
  onLogout: () => void | Promise<void>;
}) {
  const [application, setApplication] = useState<ApplicationStatus>();
  const [error, setError] = useState('');

  useEffect(() => {
    if (!session.allowed) return;
    void getApplications()
      .then((response) => {
        const match = response.applications.find((candidate) => candidate.id === applicationID);
        if (match) setApplication(match);
        else setError('That application is not in the service catalog.');
      })
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Unable to load the application'));
  }, [applicationID, session.allowed]);

  return (
    <Shell session={session} user={user} onLogout={onLogout}>
      {!sessionReady && <main className="page"><div className="stream-loading">Checking your access…</div></main>}
      {sessionReady && !session.allowed && (
        <main className="page">
          <a className="back-link" href="/">← Platform status</a>
          <AccessNotice />
        </main>
      )}
      {sessionReady && session.allowed && !application && (
        <main className="page">
          <a className="back-link" href="/">← Platform status</a>
          {error ? <div className="inline-error">{error}</div> : <div className="stream-loading">Loading application…</div>}
        </main>
      )}
      {sessionReady && session.allowed && application && <ApplicationDetail application={application} />}
    </Shell>
  );
}

const signedOut: Session = { authenticated: false, allowed: false };

export function App() {
  const [session, setSession] = useState<Session>(signedOut);
  const [sessionReady, setSessionReady] = useState(false);
  const [user, setUser] = useState<Identity>();

  useEffect(() => {
    // /v1/session always answers 200, so a failure here means the network or
    // the API is down, not that the visitor is unwelcome. Either way the public
    // status page below still renders.
    void checkSession()
      .then(setSession)
      .catch(() => setSession(signedOut))
      .finally(() => setSessionReady(true));
  }, []);

  useEffect(() => {
    if (!session.authenticated) {
      setUser(undefined);
      return;
    }
    void getIdentity().then(setUser).catch(() => setUser(undefined));
  }, [session.authenticated]);

  const onLogout = useCallback(async () => {
    await logout();
    setUser(undefined);
    setSession(signedOut);
  }, []);

  const detailMatch = window.location.pathname.match(/^\/applications\/([^/]+)\/?$/);
  if (detailMatch) {
    return (
      <ExplorerRoute
        applicationID={decodeURIComponent(detailMatch[1])}
        onLogout={onLogout}
        session={session}
        sessionReady={sessionReady}
        user={user}
      />
    );
  }

  return <StatusPage session={session} user={user} onLogout={onLogout} />;
}
