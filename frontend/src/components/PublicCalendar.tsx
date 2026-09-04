import { useMemo } from 'react';
import { addMonths, calendarDays, dateKey, dayLabel, monthLabel, occurrenceDateKey, startOfMonth, timeLabel } from '../date';
import type { Occurrence, Scope } from '../types';

interface PublicCalendarProps {
  month: Date;
  scopes: Scope[];
  occurrences: Occurrence[];
  selectedScope: string;
  loading: boolean;
  error: string;
  onMonthChange: (month: Date) => void;
  onScopeChange: (scopeId: string) => void;
  onSubscribe: () => void;
  onSelectEvent: (occurrence: Occurrence) => void;
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
  const tone = occurrence.scope_kind === 'TEAM'
    ? 'border-brand bg-brand/10 dark:bg-brand/25'
    : occurrence.scope_kind === 'PROJECT'
      ? 'border-deep bg-deep/8 dark:border-soft dark:bg-soft/15'
      : 'border-soft bg-soft/40 dark:border-soft/70 dark:bg-soft/10';
  return (
    <button type="button" onClick={onClick} className={`w-full border-l-[3px] text-left hover:brightness-95 dark:hover:brightness-110 ${tone} ${compact ? 'rounded-md px-2 py-1.5' : 'min-h-11 rounded-xl px-3 py-3'}`}>
      <span className={`block truncate font-semibold ${compact ? 'text-xs' : 'text-sm'}`}>{occurrence.title}</span>
      <span className={`mt-0.5 block truncate text-ink/65 dark:text-white/65 ${compact ? 'text-[11px]' : 'text-xs'}`}>
        {timeLabel(occurrence.starts_at, occurrence.all_day)}{compact ? '' : ` · ${occurrence.scope_name}`}
      </span>
    </button>
  );
}

export function PublicCalendar(props: PublicCalendarProps) {
  const days = calendarDays(props.month);
  const currentMonth = props.month.getUTCMonth();
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
    <main className="mx-auto w-full max-w-[1500px] px-4 pb-16 pt-8 sm:px-6 lg:px-8">
      <section className="mb-8 grid gap-5 lg:grid-cols-[1fr_auto] lg:items-end">
        <div>
          <p className="mb-2 text-xs font-bold uppercase tracking-[0.2em] text-brand dark:text-soft">Public calendar</p>
          <h1 className="max-w-3xl text-3xl font-semibold tracking-[-0.04em] sm:text-5xl">Find the room where BioTron is building.</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/65 sm:text-base dark:text-white/65">Team meetings, project sessions, and subteam work blocks. Times are shown in Waterloo time.</p>
        </div>
        <button type="button" onClick={props.onSubscribe} className="min-h-12 rounded-full bg-deep px-6 text-sm font-semibold text-white hover:bg-brand focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand">
          Subscribe to a calendar
        </button>
      </section>

      <section className="overflow-hidden rounded-3xl border border-ink/10 bg-white dark:border-white/10 dark:bg-white/[0.04]">
        <div className="flex flex-col gap-4 border-b border-ink/10 p-4 sm:p-5 dark:border-white/10 lg:flex-row lg:items-center">
          <div className="flex items-center justify-between gap-2 sm:justify-start">
            <button type="button" className="grid size-11 place-items-center rounded-full border border-ink/15 hover:bg-soft/25 dark:border-white/15 dark:hover:bg-white/10" onClick={() => props.onMonthChange(addMonths(props.month, -1))} aria-label="Previous month">←</button>
            <button type="button" className="min-h-11 rounded-full px-3 text-sm font-semibold hover:bg-soft/25 dark:hover:bg-white/10" onClick={() => props.onMonthChange(startOfMonth())}>Today</button>
            <button type="button" className="grid size-11 place-items-center rounded-full border border-ink/15 hover:bg-soft/25 dark:border-white/15 dark:hover:bg-white/10" onClick={() => props.onMonthChange(addMonths(props.month, 1))} aria-label="Next month">→</button>
            <h2 className="ml-2 truncate text-lg font-semibold sm:text-xl">{monthLabel(props.month)}</h2>
          </div>
          <label className="lg:ml-auto">
            <span className="sr-only">Filter calendar</span>
            <select value={props.selectedScope} onChange={(event) => props.onScopeChange(event.target.value)} className="min-h-11 w-full rounded-full border border-ink/15 bg-transparent px-4 text-sm font-medium outline-none focus:border-brand sm:w-auto dark:border-white/20">
              <option value="">All BioTron events</option>
              {props.scopes.map((scope) => <option key={scope.id} value={scope.id}>{scope.kind === 'TEAM' ? 'Teamwide' : scope.name}</option>)}
            </select>
          </label>
        </div>

        {props.error && <div className="border-b border-red-500/20 bg-red-50 px-5 py-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200">{props.error}</div>}
        {props.loading && <div className="h-1 animate-pulse bg-brand" aria-label="Loading calendar" />}

        <div className="hidden md:block">
          <div className="grid grid-cols-7 border-b border-ink/10 dark:border-white/10">
            {weekdayLabels.map((weekday) => <div key={weekday} className="px-3 py-2 text-xs font-bold uppercase tracking-wider text-ink/45 dark:text-white/45">{weekday}</div>)}
          </div>
          <div className="grid grid-cols-7">
            {days.map((day) => {
              const key = dateKey(day);
              const events = grouped.get(key) || [];
              const muted = day.getUTCMonth() !== currentMonth;
              return (
                <div key={key} className={`min-h-32 border-b border-r border-ink/10 p-2 dark:border-white/10 xl:min-h-40 ${muted ? 'bg-ink/[0.018] text-ink/35 dark:bg-black/10 dark:text-white/30' : ''}`}>
                  <div className={`mb-2 grid size-7 place-items-center rounded-full text-xs font-semibold ${key === today ? 'bg-deep text-white' : ''}`}>{day.getUTCDate()}</div>
                  <div className="max-h-24 space-y-1.5 overflow-y-auto pr-1 xl:max-h-32">
                    {events.map((occurrence) => <EventButton key={`${occurrence.series_id}-${occurrence.recurrence_id_local}`} occurrence={occurrence} compact onClick={() => props.onSelectEvent(occurrence)} />)}
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="divide-y divide-ink/10 dark:divide-white/10 md:hidden">
          {agendaDays.length === 0 && !props.loading ? (
            <div className="px-5 py-16 text-center">
              <p className="font-semibold">Nothing scheduled here yet.</p>
              <p className="mt-2 text-sm text-ink/60 dark:text-white/60">Try another calendar or check the next month.</p>
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
                  <p className="bg-ink/[0.03] px-4 py-2 text-xs font-bold uppercase tracking-wider text-ink/50 dark:bg-white/5 dark:text-white/50">
                    {agendaMonthFormat.format(new Date(`${key}T12:00:00Z`))}
                  </p>
                )}
                <section className="grid grid-cols-[4.5rem_1fr] gap-3 px-4 py-5">
                  <div>
                    <p className="text-xs font-bold uppercase tracking-wider text-brand dark:text-soft">{dayLabel(key).split(',')[0]}</p>
                    <p className="mt-1 text-2xl font-semibold">{Number(key.slice(-2))}</p>
                  </div>
                  <div className="min-w-0 space-y-2">
                    {(grouped.get(key) || []).map((occurrence) => <EventButton key={`${occurrence.series_id}-${occurrence.recurrence_id_local}`} occurrence={occurrence} onClick={() => props.onSelectEvent(occurrence)} />)}
                  </div>
                </section>
              </div>
            );
          })}
        </div>
      </section>
    </main>
  );
}
