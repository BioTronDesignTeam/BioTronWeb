import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, ArrowUpRight, CalendarDays, Clock3, MapPin } from 'lucide-react';
import PageMeta from '../components/PageMeta';

interface CalendarOccurrence {
  series_id: string;
  recurrence_id_local: string;
  scope_name: string;
  title: string;
  location: string;
  starts_at: string;
  ends_at: string;
  all_day: boolean;
}

const CALENDAR_API_URL = (import.meta.env.VITE_CALENDAR_API_URL || 'http://localhost:18084').replace(/\/$/, '');
const CALENDAR_URL = (import.meta.env.VITE_CALENDAR_URL || 'http://localhost:5176').replace(/\/$/, '');
const TORONTO_TIMEZONE = 'America/Toronto';

function dateKey(date: Date) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: TORONTO_TIMEZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(date);
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${values.year}-${values.month}-${values.day}`;
}

function eventDate(occurrence: CalendarOccurrence) {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: TORONTO_TIMEZONE,
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  }).format(new Date(occurrence.starts_at));
}

function eventTime(occurrence: CalendarOccurrence) {
  if (occurrence.all_day) return 'All day';
  const format = new Intl.DateTimeFormat('en-CA', {
    timeZone: TORONTO_TIMEZONE,
    hour: 'numeric',
    minute: '2-digit',
  });
  return `${format.format(new Date(occurrence.starts_at))} to ${format.format(new Date(occurrence.ends_at))}`;
}

export default function CalendarPage() {
  const [events, setEvents] = useState<CalendarOccurrence[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const range = useMemo(() => {
    const from = new Date();
    const to = new Date(from);
    to.setDate(to.getDate() + 42);
    return { from: dateKey(from), to: dateKey(to) };
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const query = new URLSearchParams(range);
        const response = await fetch(`${CALENDAR_API_URL}/v1/events?${query}`, {
          credentials: 'omit',
          signal: controller.signal,
        });
        if (!response.ok) throw new Error('calendar unavailable');
        const nextEvents = (await response.json()) as CalendarOccurrence[];
        const now = Date.now();
        setEvents(nextEvents.filter((event) => new Date(event.ends_at).getTime() > now).slice(0, 5));
      } catch (caught) {
        if (!(caught instanceof DOMException && caught.name === 'AbortError')) setError(true);
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    }
    void load();
    return () => controller.abort();
  }, [range]);

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
          <p>See the next six weeks here, or open the full calendar to browse by month and team.</p>
          <div className="calendarpage__actions">
            <a href={CALENDAR_URL} className="calendarpage__primary">
              Open full calendar <ArrowUpRight size={16} />
            </a>
            <a href={`${CALENDAR_URL}/?subscribe=1`} className="calendarpage__secondary">
              Choose subscriptions <ArrowRight size={16} />
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
            <div className="calendarpage__status">Nothing public is scheduled in the next six weeks.</div>
          )}
          {!loading && !error && events.length > 0 && (
            <ol className="calendarpage__events">
              {events.map((event) => (
                <li key={`${event.series_id}-${event.recurrence_id_local}`} className="calendarpage__event">
                  <time dateTime={event.starts_at} className="calendarpage__date">{eventDate(event)}</time>
                  <div className="calendarpage__event-main">
                    <h3>{event.title}</h3>
                    <span className="calendarpage__scope">{event.scope_name}</span>
                    <div className="calendarpage__meta">
                      <span><Clock3 size={15} aria-hidden="true" />{eventTime(event)}</span>
                      {event.location && <span><MapPin size={15} aria-hidden="true" />{event.location}</span>}
                    </div>
                  </div>
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </main>
  );
}
