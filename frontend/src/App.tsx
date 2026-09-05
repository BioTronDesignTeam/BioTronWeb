import {
  FormEvent,
  KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type CSSProperties,
  type ReactNode,
} from 'react';
import { Brand, Button, ThemeToggle, UserMenu } from '@biotron/style';
import {
  type ApplicationStatus,
  type HealthState,
  type HistoryBucket,
  type Identity,
  type LogEntry,
  type LogLevel,
  type Session,
  type StatusApplication,
  type StatusHistoryResponse,
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
const longDateFormatter = new Intl.DateTimeFormat(undefined, {
  weekday: 'short', month: 'short', day: 'numeric', year: 'numeric',
});
const rangeFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
const monthFormatter = new Intl.DateTimeFormat(undefined, { month: 'long', year: 'numeric' });
const weekdayFormatter = new Intl.DateTimeFormat(undefined, { weekday: 'short' });

type LogMode = 'recent' | 'history';

/**
 * Status is never conveyed by colour alone: every state carries its own glyph
 * shape and a word as well, so the page still reads correctly in monochrome or
 * to anyone who cannot separate the hues.
 */
const statusMeta: Record<StatusState, { label: string; headline: string; description: string }> = {
  operational: {
    label: 'Operational',
    headline: 'All systems operational',
    description: 'We are not aware of any issues affecting the platform.',
  },
  degraded: {
    label: 'Degraded',
    headline: 'Some systems are degraded',
    description: 'One or more components are unavailable or not being observed. Expand an application below for detail.',
  },
  down: {
    label: 'Down',
    headline: 'Major outage',
    description: 'Every component is currently failing its health check.',
  },
  unknown: {
    label: 'No data',
    headline: 'Status is not being observed',
    description: 'Logger has no recent health checks to report.',
  },
};

const healthMeta: Record<HealthState, { label: string; state: StatusState }> = {
  healthy: { label: 'Operational', state: 'operational' },
  unhealthy: { label: 'Unavailable', state: 'down' },
  unknown: { label: 'Awaiting check', state: 'unknown' },
};

function relativeTime(timestamp?: string | null) {
  if (!timestamp) return 'not checked yet';
  const seconds = Math.round((new Date(timestamp).getTime() - Date.now()) / 1000);
  if (Math.abs(seconds) < 60) return relativeFormatter.format(seconds, 'second');
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return relativeFormatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return relativeFormatter.format(hours, 'hour');
  return relativeFormatter.format(Math.round(hours / 24), 'day');
}

/** "100%", "99.43%", never "100.00%": trailing zeros only add noise. */
function percent(value: number) {
  return `${Number(value.toFixed(2))}%`;
}

/** A null uptime means the window was never observed, which is not 100%. */
function uptimeText(value: number | null | undefined) {
  return value === null || value === undefined ? 'No data' : `${percent(value)} uptime`;
}

function localNoon(date: string) {
  return new Date(`${date}T12:00:00`);
}

/* Icons ------------------------------------------------------------------ */

const glyphs: Record<StatusState, ReactNode> = {
  operational: (
    <>
      <circle cx="8" cy="8" r="8" fill="currentColor" />
      <path d="M4.6 8.3l2.2 2.2 4.6-4.8" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
    </>
  ),
  degraded: (
    <>
      <path d="M8 1.2L15.4 14.4H.6z" fill="currentColor" strokeLinejoin="round" />
      <path d="M8 5.6v4" fill="none" stroke="#fff" strokeWidth="1.7" strokeLinecap="round" />
      <circle cx="8" cy="12" r="1" fill="#fff" />
    </>
  ),
  down: (
    <>
      <rect x="0.5" y="0.5" width="15" height="15" rx="3.5" fill="currentColor" />
      <path d="M5.4 5.4l5.2 5.2M10.6 5.4l-5.2 5.2" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" />
    </>
  ),
  unknown: (
    <>
      <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" strokeWidth="1.5" strokeDasharray="2.6 2.1" />
      <path d="M6.2 6.4a1.8 1.8 0 1 1 2.6 1.6c-.6.3-.8.6-.8 1.2" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      <circle cx="8" cy="11.5" r=".9" fill="currentColor" />
    </>
  ),
};

function StatusIcon({ state, label }: { state: StatusState; label?: string }) {
  return (
    <span className={`status-icon status-icon--${state}`}>
      <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">{glyphs[state]}</svg>
      <span className="visually-hidden">{label ?? statusMeta[state].label}</span>
    </span>
  );
}

function Chevron({ open }: { open: boolean }) {
  return (
    <svg className={`chevron ${open ? 'chevron--open' : ''}`} width="10" height="6" viewBox="0 0 10 6" aria-hidden="true" focusable="false">
      <path d="M1.5 1l3.75 4L9 1" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function InfoIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true" focusable="false">
      <path strokeLinecap="round" strokeLinejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
    </svg>
  );
}

function CalendarIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true" focusable="false">
      <path strokeLinecap="round" strokeLinejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" />
    </svg>
  );
}

/**
 * The description sits behind an "i" the way it does on every hosted status
 * page, so a row stays one line tall. It opens on hover and on focus, and on a
 * tap, because a tapped button is focused.
 */
function InfoTip({ id, name, text }: { id: string; name: string; text: string }) {
  return (
    <span className="info-tip">
      <button type="button" className="info-tip__trigger" aria-label={`About ${name}`} aria-describedby={id}>
        <InfoIcon />
      </button>
      <span role="tooltip" id={id} className="popover info-tip__bubble">{text}</span>
    </span>
  );
}

/* Ninety-day bar --------------------------------------------------------- */

/**
 * How many days of the bar fit without the pills turning into hairlines. A
 * phone gets thirty, a tablet sixty. The bar is a flex row, so no width of
 * viewport can ever make the page body scroll sideways.
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

function dayDetail(bucket: HistoryBucket) {
  switch (bucket.state) {
    case 'operational':
      return 'No incidents';
    case 'degraded':
      return `Degraded · ${uptimeText(bucket.uptime)}`;
    case 'down':
      return `Outage · ${uptimeText(bucket.uptime)}`;
    default:
      return 'No data · nobody was watching';
  }
}

/**
 * One pill per day. Pointing at a pill, or focusing the bar and using the
 * arrow keys, opens a popover naming the day and what happened on it, so the
 * bar is more than a decorative stripe.
 */
function UptimeBar({ buckets, days, label }: { buckets: HistoryBucket[]; days: number; label: string }) {
  const shown = buckets.slice(-days);
  const [active, setActive] = useState<number | null>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const summary = `${label}, ${shown.length}-day history: ` +
    `${shown.filter((bucket) => bucket.state === 'operational').length} fully operational days, ` +
    `${shown.filter((bucket) => bucket.state === 'degraded' || bucket.state === 'down').length} with incidents, ` +
    `${shown.filter((bucket) => bucket.state === 'unknown').length} without data. Use the arrow keys to read each day.`;

  const indexAt = (clientX: number) => {
    const track = trackRef.current;
    if (!track || shown.length === 0) return null;
    const rect = track.getBoundingClientRect();
    if (rect.width === 0) return null;
    const ratio = (clientX - rect.left) / rect.width;
    return Math.min(shown.length - 1, Math.max(0, Math.floor(ratio * shown.length)));
  };

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (shown.length === 0) return;
    const current = active ?? shown.length - 1;
    const moves: Record<string, number> = {
      ArrowLeft: Math.max(0, current - 1),
      ArrowRight: Math.min(shown.length - 1, current + 1),
      Home: 0,
      End: shown.length - 1,
    };
    if (event.key in moves) {
      event.preventDefault();
      setActive(moves[event.key]);
    } else if (event.key === 'Escape') {
      setActive(null);
    }
  }

  const bucket = active === null ? null : shown[active];

  return (
    <div className="uptime-bar">
      <div
        ref={trackRef}
        className="uptime-bar__track"
        role="group"
        aria-label={summary}
        tabIndex={0}
        onPointerMove={(event) => setActive(indexAt(event.clientX))}
        onPointerLeave={() => setActive(null)}
        onFocus={() => setActive((current) => current ?? shown.length - 1)}
        onBlur={() => setActive(null)}
        onKeyDown={onKeyDown}
      >
        {shown.map((day, index) => (
          <span
            className={`uptime-day uptime-day--${day.state} ${index === active ? 'uptime-day--active' : ''}`}
            key={day.date}
          />
        ))}
      </div>
      <div className="uptime-bar__live" aria-live="polite">
        {bucket && active !== null && (
          <div
            className="popover day-popover"
            style={{ '--x': `${((active + 0.5) / shown.length) * 100}%` } as CSSProperties}
          >
            <div className="day-popover__date">{longDateFormatter.format(localNoon(bucket.date))}</div>
            <div className={`day-popover__line day-popover__line--${bucket.state}`}>
              <StatusIcon state={bucket.state} label="" />
              <span>{dayDetail(bucket)}</span>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* Shared chrome ---------------------------------------------------------- */

function Shell({
  children,
  session,
  user,
  onLogout,
  narrow = false,
}: {
  children: ReactNode;
  session: Session;
  user?: Identity;
  onLogout: () => void | Promise<void>;
  narrow?: boolean;
}) {
  return (
    <div className="shell">
      <header className={`topbar ${narrow ? 'topbar--narrow' : ''}`}>
        <a className="brand" href="/">
          <Brand className="brand__mark" />
          <span className="brand__divider" aria-hidden="true" />
          <span className="brand__name">Status Logger</span>
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

/* Public status data ----------------------------------------------------- */

/**
 * Both requests are unauthenticated, so this runs identically for a signed-out
 * visitor and never depends on a session being resolved first.
 */
function usePlatformStatus(poll: boolean) {
  const [status, setStatus] = useState<StatusResponse>();
  const [history, setHistory] = useState<StatusHistoryResponse>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const [current, past] = await Promise.all([getStatus(), getStatusHistory(HISTORY_DAYS)]);
      setStatus(current);
      setHistory(past);
      setError('');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Status is unavailable');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    if (!poll) return;
    const timer = window.setInterval(() => void refresh(), 30000);
    return () => window.clearInterval(timer);
  }, [poll, refresh]);

  return { status, history, error, loading };
}

/** "Jun 8 – Sep 5, 2026": the days the bars actually cover. */
function rangeLabel(history?: StatusHistoryResponse) {
  const buckets = history?.applications[0]?.buckets ?? history?.components[0]?.buckets;
  if (!buckets || buckets.length === 0) return '';
  return rangeFormatter.formatRange(localNoon(buckets[0].date), localNoon(buckets[buckets.length - 1].date));
}

/* Status page ------------------------------------------------------------ */

function StatusBanner({ overall, loading }: { overall?: StatusResponse['overall']; loading: boolean }) {
  const state = overall?.state ?? 'unknown';
  const meta = statusMeta[state];
  return (
    <section className={`box box--${overall ? state : 'pending'}`} aria-live="polite">
      <div className="box__head">
        {overall && <StatusIcon state={state} label="" />}
        <h1>{loading && !overall ? 'Checking platform status…' : meta.headline}</h1>
      </div>
      <div className="box__body">
        <p>{overall ? meta.description : 'Health is checked from inside the BioTron service network.'}</p>
        {overall && (
          <p className="box__meta">
            Updated {relativeTime(overall.updated_at)} · {uptimeText(overall.uptime_90d)} over {HISTORY_DAYS} days
          </p>
        )}
      </div>
    </section>
  );
}

function ComponentRow({
  component,
  buckets,
  days,
}: {
  component: StatusApplication['components'][number];
  buckets: HistoryBucket[];
  days: number;
}) {
  return (
    <div className="component">
      <div className="component__row">
        <StatusIcon state={component.state} />
        <h4 className="component__name">{component.name}</h4>
        <span className="row__uptime">{uptimeText(component.uptime_90d)}</span>
      </div>
      {buckets.length > 0 && <UptimeBar buckets={buckets} days={days} label={component.name} />}
    </div>
  );
}

/**
 * One row per application. Collapsed, it is a name, a figure, and a bar. The
 * "N components" control unfolds the parts beneath it, each with its own bar,
 * so the whole platform fits on one screen until someone asks for more.
 */
function ApplicationGroup({
  application,
  applicationBuckets,
  componentBuckets,
  days,
  explorable,
}: {
  application: StatusApplication;
  applicationBuckets: HistoryBucket[];
  componentBuckets: Map<string, HistoryBucket[]>;
  days: number;
  explorable: boolean;
}) {
  const [open, setOpen] = useState(false);
  const count = application.components.length;
  const panelID = `components-${application.id}`;

  return (
    <div className="group">
      <div className="group__row">
        <StatusIcon state={application.state} />
        <h3 className="group__name">{application.name}</h3>
        <InfoTip id={`about-${application.id}`} name={application.name} text={application.description} />
        <button
          type="button"
          className="group__toggle"
          aria-expanded={open}
          aria-controls={panelID}
          onClick={() => setOpen((current) => !current)}
        >
          <span className="group__count">{count} {count === 1 ? 'component' : 'components'}</span>
          <Chevron open={open} />
        </button>
        <span className="row__uptime">{uptimeText(application.uptime_90d)}</span>
      </div>

      {open && (
        <div className="group__components" id={panelID}>
          {application.components.map((component) => (
            <ComponentRow
              buckets={componentBuckets.get(component.id) ?? []}
              component={component}
              days={days}
              key={component.id}
            />
          ))}
          {explorable && (
            <a className="group__logs" href={`/applications/${application.id}`}>
              Open logs <span aria-hidden="true">→</span>
            </a>
          )}
        </div>
      )}

      {applicationBuckets.length > 0 && (
        <UptimeBar buckets={applicationBuckets} days={days} label={application.name} />
      )}
    </div>
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
  const { status, history, error, loading } = usePlatformStatus(true);
  const days = useVisibleDays();

  const applicationBuckets = useMemo(() => {
    const index = new Map<string, HistoryBucket[]>();
    history?.applications.forEach((application) => index.set(application.id, application.buckets));
    return index;
  }, [history]);
  const componentBuckets = useMemo(() => {
    const index = new Map<string, HistoryBucket[]>();
    history?.components.forEach((component) => index.set(component.id, component.buckets));
    return index;
  }, [history]);

  return (
    <Shell session={session} user={user} onLogout={onLogout} narrow>
      <main className="column">
        {session.authenticated && !session.allowed && <AccessNotice />}

        <StatusBanner overall={status?.overall} loading={loading} />

        {error && <div className="inline-error">{error}</div>}

        <section className="box" aria-labelledby="system-status">
          <div className="box__head box__head--split">
            <h2 id="system-status">System status</h2>
            {history && <span className="box__range">{rangeLabel(history)}</span>}
          </div>
          <div className="box__rows">
            {status?.applications.map((application) => (
              <ApplicationGroup
                application={application}
                applicationBuckets={applicationBuckets.get(application.id) ?? []}
                componentBuckets={componentBuckets}
                days={days}
                explorable={session.allowed}
                key={application.id}
              />
            ))}
            {!status && !error && <div className="box__placeholder">Loading components…</div>}
          </div>
        </section>

        <div className="column__actions">
          <a className="button button--secondary" href="/history">
            <CalendarIcon /> View history
          </a>
        </div>

        <p className="column__footnote">
          Health is checked every 15 seconds from inside the BioTron service network. Uptime is
          time-weighted across every component's observed time; days nobody was watching are
          hatched and left out of the figures.
        </p>
      </main>
    </Shell>
  );
}

/* History page ----------------------------------------------------------- */

type Incident = {
  key: string;
  application: string;
  component: string;
  state: 'degraded' | 'down';
  uptime: number | null;
};

type IncidentDay = { date: string; incidents: Incident[] };
type IncidentMonth = { key: string; label: string; days: IncidentDay[] };

/**
 * Past incidents, read back out of the same daily history the bars are drawn
 * from. Logger has no separate incident record, so a day counts as an incident
 * when a component spent part of it degraded or down. A day with no data is not
 * an incident: nobody was watching, which is a different claim.
 */
function incidentMonths(history: StatusHistoryResponse | undefined, applications: StatusApplication[]): IncidentMonth[] {
  if (!history) return [];
  // Component names repeat across applications: several are called "Web" and
  // several "API". An incident line has to name the application too, or it
  // says nothing about what was down.
  const applicationNames = new Map(applications.map((application) => [application.id, application.name]));
  const byDate = new Map<string, Incident[]>();
  for (const component of history.components) {
    const application = applicationNames.get(component.application_id) ?? component.application_id;
    for (const bucket of component.buckets) {
      if (bucket.state !== 'degraded' && bucket.state !== 'down') continue;
      const list = byDate.get(bucket.date) ?? [];
      list.push({
        key: `${bucket.date}-${component.id}`,
        application,
        component: component.name,
        state: bucket.state,
        uptime: bucket.uptime,
      });
      byDate.set(bucket.date, list);
    }
  }

  const months: IncidentMonth[] = [];
  const dates = Array.from(byDate.keys()).sort().reverse();
  for (const date of dates) {
    const key = date.slice(0, 7);
    let month = months[months.length - 1];
    if (!month || month.key !== key) {
      month = { key, label: monthFormatter.format(localNoon(date)), days: [] };
      months.push(month);
    }
    month.days.push({ date, incidents: byDate.get(date) ?? [] });
  }
  return months;
}

function HistoryPage() {
  const { status, history, error, loading } = usePlatformStatus(false);
  const months = useMemo(() => incidentMonths(history, status?.applications ?? []), [history, status]);

  return (
    <main className="column">
      <nav className="crumbs" aria-label="Breadcrumb">
        <a href="/">BioTron</a>
        <span aria-hidden="true">/</span>
        <span aria-current="page">History</span>
      </nav>

      <div className="history__head">
        <h1>History</h1>
        {history && <span className="box__range">{rangeLabel(history)}</span>}
      </div>

      {error && <div className="inline-error">{error}</div>}
      {loading && !history && <p className="history__empty">Loading history…</p>}
      {history && months.length === 0 && (
        <p className="history__empty">No incidents in the past {history.days} days.</p>
      )}

      {months.map((month) => (
        <section className="month" key={month.key} aria-labelledby={`month-${month.key}`}>
          <h2 className="month__name" id={`month-${month.key}`}>{month.label}</h2>
          {month.days.map((day) => (
            <div className="day" key={day.date}>
              <div className="day__date">
                <strong>{day.date.slice(8, 10)}</strong>
                <span>{weekdayFormatter.format(localNoon(day.date))}</span>
              </div>
              <ul className="day__incidents">
                {day.incidents.map((incident) => (
                  <li className={`incident incident--${incident.state}`} key={incident.key}>
                    <span className="incident__rail" aria-hidden="true" />
                    <div className="incident__body">
                      <div className="incident__title">
                        <span>{incident.application} · {incident.component}</span>
                        <span className="incident__uptime">{uptimeText(incident.uptime)}</span>
                      </div>
                      <p className="incident__detail">
                        <StatusIcon state={incident.state} />
                        {incident.state === 'down'
                          ? 'Health checks failed repeatedly; the component was unavailable for part of the day.'
                          : 'Health checks failed briefly; the component was available for the rest of the day.'}
                      </p>
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </section>
      ))}
    </main>
  );
}

/* Log explorer (logger/view only) ---------------------------------------- */

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

function HealthBadge({ state }: { state: HealthState }) {
  const meta = healthMeta[state];
  return (
    <span className={`health-badge health-badge--${meta.state}`}>
      <StatusIcon state={meta.state} label="" />
      <span>{meta.label}</span>
    </span>
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
            <small>Checked {relativeTime(component.checked_at)}</small>
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

/* Routing ---------------------------------------------------------------- */

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

  const path = window.location.pathname;
  const detailMatch = path.match(/^\/applications\/([^/]+)\/?$/);
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

  if (/^\/history\/?$/.test(path)) {
    return (
      <Shell session={session} user={user} onLogout={onLogout} narrow>
        <HistoryPage />
      </Shell>
    );
  }

  return <StatusPage session={session} user={user} onLogout={onLogout} />;
}
