import { useEffect, useState } from 'react';
import { ArrowUpRight, CalendarDays } from 'lucide-react';
import PageMeta from '../components/PageMeta';

/**
 * The fields this view reads from the calendar's occurrence JSON. The calendar
 * API owns the shape and the selection rules; this page only renders what it
 * sends back, so keep the list to what the markup below actually uses.
 */
interface CalendarOccurrence {
  series_id: string;
  recurrence_id_local: string;
  scope_name: string;
  title: string;
  description: string;
  location: string;
  starts_at: string;
  ends_at: string;
  all_day: boolean;
}

const CALENDAR_API_URL = (import.meta.env.VITE_CALENDAR_API_URL || 'http://localhost:8083').replace(/\/$/, '');
const CALENDAR_URL = (import.meta.env.VITE_CALENDAR_URL || 'http://localhost:5176').replace(/\/$/, '');
const TORONTO_TIMEZONE = 'America/Toronto';

/** How many upcoming occurrences to show, and how far ahead to look. */
const UPCOMING_LIMIT = 5;
const UPCOMING_DAYS = 42;

const DATE_FORMAT = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE,
  weekday: 'short',
  month: 'short',
  day: 'numeric',
});

const TIME_FORMAT = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE,
  hour: 'numeric',
  minute: '2-digit',
});

function eventDate(occurrence: CalendarOccurrence) {
  return DATE_FORMAT.format(new Date(occurrence.starts_at));
}

/**
 * Drops the repeated meridiem from a range inside one half of the day, so
 * "6:00 PM to 8:30 PM" reads "6:00 to 8:30 PM". The row puts the date and the
 * time on one line, and the shorter form keeps that line off the title.
 */
function eventTime(occurrence: CalendarOccurrence) {
  if (occurrence.all_day) return 'All day';
  const from = TIME_FORMAT.format(new Date(occurrence.starts_at));
  const to = TIME_FORMAT.format(new Date(occurrence.ends_at));
  const meridiem = from.slice(-2);
  if (meridiem === to.slice(-2)) return `${from.slice(0, -3)} to ${to}`;
  return `${from} to ${to}`;
}

function eventWhen(occurrence: CalendarOccurrence) {
  return `${eventDate(occurrence)} \u00b7 ${eventTime(occurrence)}`;
}

function isTimestamp(value: unknown): value is string {
  return typeof value === 'string' && Number.isFinite(Date.parse(value));
}

function isOccurrence(value: unknown): value is CalendarOccurrence {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.series_id === 'string' &&
    typeof candidate.recurrence_id_local === 'string' &&
    typeof candidate.scope_name === 'string' &&
    typeof candidate.title === 'string' &&
    typeof candidate.description === 'string' &&
    typeof candidate.location === 'string' &&
    typeof candidate.all_day === 'boolean' &&
    isTimestamp(candidate.starts_at) &&
    isTimestamp(candidate.ends_at)
  );
}

/**
 * Validates the response at runtime. A renamed or dropped field throws here,
 * so the page falls back to its unavailable state instead of rendering
 * `undefined` and `Invalid Date`.
 */
function parseOccurrences(payload: unknown): CalendarOccurrence[] {
  if (!Array.isArray(payload)) throw new Error('calendar response is not a list');
  if (!payload.every(isOccurrence)) throw new Error('calendar response has an unexpected shape');
  return payload;
}

export default function CalendarPage() {
  const [events, setEvents] = useState<CalendarOccurrence[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    // The calendar API owns the window, the ordering, and the cut-off. This
    // page asks for the next few events and renders exactly what it returns.
    const controller = new AbortController();
    let cancelled = false;

    async function load() {
      try {
        const query = new URLSearchParams({
          limit: String(UPCOMING_LIMIT),
          days: String(UPCOMING_DAYS),
        });
        const response = await fetch(`${CALENDAR_API_URL}/v1/events/upcoming?${query}`, {
          credentials: 'omit',
          signal: controller.signal,
        });
        if (!response.ok) throw new Error('calendar unavailable');
        const occurrences = parseOccurrences(await response.json());
        if (cancelled) return;
        setEvents(occurrences);
      } catch {
        if (cancelled) return;
        setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, []);

  return (
    <main id="main" className="calendarpage">
      <PageMeta
        title="Calendar | Biotron"
        description="See upcoming Biotron meetings and subscribe to team, project, or subteam calendars."
      />
      <div className="container calendarpage__inner">
        <section className="calendarpage__intro">
          <div className="calendarpage__icon" aria-hidden="true">
            <CalendarDays size={28} />
          </div>
          <span className="eyebrow mono-label">Public calendar</span>
          <h1>Find the next place we are building.</h1>
          <p>See upcoming events here or open the full calendar.</p>
          <div className="calendarpage__actions">
            <a href={CALENDAR_URL} className="calendarpage__primary">
              Open full calendar <ArrowUpRight size={16} />
            </a>
          </div>
        </section>

        <section className="calendarpage__upcoming" aria-labelledby="upcoming-events-title">
          <div className="calendarpage__upcoming-head">
            <div>
              <span className="eyebrow mono-label">Next up</span>
              <h2 id="upcoming-events-title">Upcoming events</h2>
            </div>
            <span className="calendarpage__timezone">Waterloo time</span>
          </div>

          {loading && <div className="calendarpage__status">Loading the calendar...</div>}
          {error && !loading && (
            <div className="calendarpage__status">
              The live calendar is unavailable right now. The full calendar may still be reachable.
            </div>
          )}
          {!loading && !error && events.length === 0 && (
            <div className="calendarpage__status">Nothing public is scheduled right now.</div>
          )}
          {!loading && !error && events.length > 0 && (
            <ol className="calendarpage__events">
              {events.map((event) => (
                <li key={`${event.series_id}-${event.recurrence_id_local}`} className="calendarpage__event">
                  <h3 className="calendarpage__title">
                    {event.title}
                    <span className="calendarpage__scope">{event.scope_name}</span>
                  </h3>
                  <time dateTime={event.starts_at} className="calendarpage__when">
                    {eventWhen(event)}
                  </time>
                  <p className="calendarpage__desc">{event.description}</p>
                  <span className="calendarpage__where">{event.location}</span>
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </main>
  );
}
