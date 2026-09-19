import { useEffect, useState } from 'react';
import { ArrowUpRight, CalendarDays, Clock, MapPin } from 'lucide-react';
import PageMeta from '../components/PageMeta';

/**
 * The fields this view reads from the calendar's occurrence JSON. The calendar
 * API owns the shape and the selection rules; this page only renders what it
 * sends back, so keep the list to what the markup below actually uses.
 */
interface CalendarOccurrence {
  series_id: string;
  recurrence_id_local: string;
  title: string;
  location: string;
  starts_at: string;
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

/** The weekday, the day of the month, and the month, as the card's date block stacks them. */
function eventDay(occurrence: CalendarOccurrence) {
  const parts = Object.fromEntries(DATE_FORMAT.formatToParts(new Date(occurrence.starts_at)).map((part) => [part.type, part.value]));
  return { weekday: parts.weekday, day: parts.day, month: parts.month };
}

function eventStart(occurrence: CalendarOccurrence) {
  return occurrence.all_day ? 'All day' : TIME_FORMAT.format(new Date(occurrence.starts_at));
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
    typeof candidate.title === 'string' &&
    typeof candidate.location === 'string' &&
    typeof candidate.all_day === 'boolean' &&
    isTimestamp(candidate.starts_at)
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
        title="Calendar | BioTron"
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
              {events.map((event) => {
                const day = eventDay(event);
                return (
                  // A card carries four things: the title, the day, the start time, and the place.
                  <li key={`${event.series_id}-${event.recurrence_id_local}`} className="calendarpage__event">
                    <time dateTime={event.starts_at} className="calendarpage__date">
                      <span className="calendarpage__weekday">{day.weekday}</span>
                      <span className="calendarpage__day tabular">{day.day}</span>
                      <span className="calendarpage__month">{day.month}</span>
                    </time>
                    <div className="calendarpage__body">
                      <h3 className="calendarpage__title">{event.title}</h3>
                      <p className="calendarpage__meta">
                        <Clock size={14} aria-hidden="true" />
                        {eventStart(event)}
                      </p>
                      {event.location && (
                        <p className="calendarpage__meta">
                          <MapPin size={14} aria-hidden="true" />
                          {event.location}
                        </p>
                      )}
                    </div>
                  </li>
                );
              })}
            </ol>
          )}
        </section>
      </div>
    </main>
  );
}
