import { useMemo } from 'react';
import { dateKey, dayLabel, occurrenceDateKey, timeLabel, viewDays, viewLabel, type CalendarView } from '../date';
import { eventTone } from '../eventTone';
import type { Occurrence } from '../types';
import { TimeGrid } from './TimeGrid';

interface PublicCalendarProps {
  view: CalendarView;
  /** A day inside the range on screen. The month view draws the month it falls in. */
  anchor: Date;
  occurrences: Occurrence[];
  loading: boolean;
  error: string;
  onSelectEvent: (occurrence: Occurrence) => void;
  onOpenDay: (day: Date) => void;
}

interface CalendarToolbarProps {
  view: CalendarView;
  anchor: Date;
  /** Back or forward by one day, week, or month, whichever the view shows. */
  onStep: (direction: -1 | 1) => void;
  /** The sidebar is narrow, so its copy is tighter and spans the full width. */
  inSidebar?: boolean;
}

const weekdayLabels = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

const todayParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'America/Toronto', year: 'numeric', month: '2-digit', day: '2-digit',
});

const agendaMonthFormat = new Intl.DateTimeFormat('en-CA', { month: 'long', year: 'numeric', timeZone: 'UTC' });

function todayKey() {
  const values = Object.fromEntries(todayParts.formatToParts(new Date()).map((part) => [part.type, part.value]));
  return `${values.year}-${values.month}-${values.day}`;
}

function EventButton({ occurrence, onClick, compact = false }: { occurrence: Occurrence; onClick: () => void; compact?: boolean }) {
  const tone = eventTone(occurrence);
  return (
    <button type="button" onClick={onClick} className={`w-full border-l-[3px] text-left hover:brightness-95 dark:hover:brightness-110 ${tone} ${compact ? 'rounded-md px-2 py-1.5' : 'min-h-11 rounded-xl px-3 py-3'}`}>
      <span className={`block truncate font-semibold ${compact ? 'text-xs' : 'text-sm'}`}>{occurrence.title}</span>
      <span className={`mt-0.5 block truncate text-ink/65 dark:text-muted ${compact ? 'text-[11px]' : 'text-xs'}`}>
        {timeLabel(occurrence.starts_at, occurrence.all_day)}{compact ? '' : ` · ${occurrence.scope_path}`}
      </span>
    </button>
  );
}

const arrowButton = 'grid shrink-0 place-items-center rounded-full border border-ink/15 hover:bg-soft/25 dark:border-line-strong dark:hover:bg-white/10';

/** The arrows and the name of the range on screen. The sidebar shows it, or the page header when the sidebar is not docked. */
export function CalendarToolbar({ view, anchor, onStep, inSidebar = false }: CalendarToolbarProps) {
  const arrow = `${arrowButton} ${inSidebar ? 'size-10' : 'size-11'}`;
  const label = viewLabel(view, anchor);
  // A week that straddles two years is the longest label, and the sidebar is narrow.
  const size = inSidebar ? (label.length > 16 ? 'text-sm' : 'text-base') : 'min-w-[8.75rem] text-base sm:min-w-[10.5rem] sm:text-xl';
  return (
    // The label sits between the arrows. Its box has a fixed width so that a
    // shorter name does not slide the next button out from under the pointer.
    <div className={`flex items-center ${inSidebar ? 'justify-between' : 'gap-1 sm:gap-2'}`}>
      <button type="button" className={arrow} onClick={() => onStep(-1)} aria-label={`Previous ${view}`}>←</button>
      <h2 className={`text-center font-semibold ${size}`}>{label}</h2>
      <button type="button" className={arrow} onClick={() => onStep(1)} aria-label={`Next ${view}`}>→</button>
    </div>
  );
}

export function PublicCalendar(props: PublicCalendarProps) {
  const { view, anchor } = props;
  const days = useMemo(() => viewDays(view, anchor), [view, anchor]);
  // The month the agenda takes as read, so it labels only the days outside it.
  const currentMonth = (view === 'month' ? anchor : days[0]).getUTCMonth();
  const today = todayKey();

  const grouped = useMemo(() => {
    const byDay = new Map<string, Occurrence[]>();
    for (const occurrence of props.occurrences) {
      const key = occurrenceDateKey(occurrence);
      byDay.set(key, [...(byDay.get(key) || []), occurrence]);
    }
    return byDay;
  }, [props.occurrences]);

  // The agenda is the only view below md, and it used to be filtered to the
  // anchor month while the fetch and the desktop grid both cover the whole
  // six-week window. A kickoff on Wed 1 April sitting in the March grid was
  // visible on every desktop and on no phone, and an empty anchor month made
  // the page claim nothing was scheduled while events were on screen beside it.
  const agendaDays = useMemo(() => {
    const window = new Set(days.map(dateKey));
    return [...grouped.keys()].filter((key) => window.has(key)).sort();
  }, [days, grouped]);

  return (
    // The page never scrolls: this column takes the height left under the
    // header, and the month grid divides it between the weeks.
    <main className="flex min-h-0 min-w-0 flex-1 flex-col">
      <h1 className="sr-only">BioTron public calendar</h1>
      {/* The month controls live in the page header, so the grid starts right under it. */}
      <section className="flex min-h-0 flex-1 flex-col bg-white dark:bg-surface">
        {props.error && <div className="border-b border-red-500/20 bg-red-50 px-5 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{props.error}</div>}
        {props.loading && <div className="h-1 animate-pulse bg-brand" aria-label="Loading calendar" />}

        {/* A week needs seven columns, which a phone cannot give: below md the
            week falls back to the agenda list. One day fits at any width. */}
        {view !== 'month' && (
          <div className={`min-h-0 flex-1 flex-col ${view === 'week' ? 'hidden md:flex' : 'flex'}`}>
            <TimeGrid days={days} occurrences={props.occurrences} today={today} onSelectEvent={props.onSelectEvent} onOpenDay={view === 'week' ? props.onOpenDay : undefined} />
          </div>
        )}

        {view === 'month' && <div className="hidden min-h-0 flex-1 flex-col overflow-y-auto md:flex">
          <div className="grid shrink-0 grid-cols-7 border-b border-ink/10 dark:border-line">
            {weekdayLabels.map((weekday) => <div key={weekday} className="px-3 py-2 text-xs font-bold uppercase tracking-wider text-ink/45 dark:text-faint">{weekday}</div>)}
          </div>
          {/* Every week gets an equal share of the height. The floor keeps a
              day readable in a short window, where this block scrolls instead. */}
          <div className="grid min-h-0 flex-1 grid-cols-7" style={{ gridTemplateRows: `repeat(${days.length / 7}, minmax(5.5rem, 1fr))` }}>
            {days.map((day) => {
              const key = dateKey(day);
              const events = grouped.get(key) || [];
              const muted = day.getUTCMonth() !== currentMonth;
              return (
                <div key={key} className={`flex min-h-0 flex-col overflow-hidden border-b border-r border-ink/10 p-2 dark:border-line ${muted ? 'bg-ink/[0.018] text-ink/35 dark:bg-black/10 dark:text-faint' : ''}`}>
                  <button type="button" onClick={() => props.onOpenDay(day)} aria-label={`Open ${key}`} className={`mb-1 grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold ${key === today ? 'bg-deep text-white dark:bg-brand' : 'hover:bg-soft/30 dark:hover:bg-white/10'}`}>{day.getUTCDate()}</button>
                  <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1">
                    {events.map((occurrence) => <EventButton key={`${occurrence.series_id}-${occurrence.recurrence_id_local}`} occurrence={occurrence} compact onClick={() => props.onSelectEvent(occurrence)} />)}
                  </div>
                </div>
              );
            })}
          </div>
        </div>}

        {view !== 'day' && <div className="min-h-0 flex-1 divide-y divide-ink/10 overflow-y-auto pb-[env(safe-area-inset-bottom,0px)] dark:divide-line md:hidden">
          {agendaDays.length === 0 && !props.loading ? (
            <div className="px-5 py-16 text-center">
              <p className="font-semibold">Nothing scheduled here yet.</p>
              <p className="mt-2 text-sm text-ink/60 dark:text-muted">Try another calendar or check the next {view}.</p>
            </div>
          ) : agendaDays.map((key, index) => {
            const dayMonth = Number(key.slice(5, 7)) - 1;
            const previousMonth = index === 0 ? null : Number(agendaDays[index - 1].slice(5, 7)) - 1;
            // The window spills into the months either side, so label the
            // change rather than leaving two bare "1"s next to each other.
            const showMonth = dayMonth !== currentMonth && dayMonth !== previousMonth;
            return (
              <div key={key}>
                {showMonth && (
                  <p className="bg-ink/[0.03] px-4 py-2 text-xs font-bold uppercase tracking-wider text-ink/50 dark:bg-surface-2 dark:text-muted">
                    {agendaMonthFormat.format(new Date(`${key}T12:00:00Z`))}
                  </p>
                )}
                <section className="grid grid-cols-[4.5rem_1fr] gap-3 px-4 py-5">
                  <div>
                    <p className="text-xs font-bold uppercase tracking-wider text-brand dark:text-link">{dayLabel(key).split(',')[0]}</p>
                    <p className="mt-1 text-2xl font-semibold">{Number(key.slice(-2))}</p>
                  </div>
                  <div className="min-w-0 space-y-2">
                    {(grouped.get(key) || []).map((occurrence) => <EventButton key={`${occurrence.series_id}-${occurrence.recurrence_id_local}`} occurrence={occurrence} onClick={() => props.onSelectEvent(occurrence)} />)}
                  </div>
                </section>
              </div>
            );
          })}
        </div>}
      </section>
    </main>
  );
}
