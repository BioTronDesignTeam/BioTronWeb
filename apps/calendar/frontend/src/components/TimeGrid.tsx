import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { addDays, dateKey, dayLabel, timeLabel, torontoClock, weekdayLabel } from '../date';
import { eventTone } from '../eventTone';
import type { Occurrence } from '../types';
import type { EventDraft } from './EventEditor';
import { useEventHover } from '../eventHover';

interface TimeGridProps {
  days: Date[];
  occurrences: Occurrence[];
  today: string;
  onSelectEvent: (occurrence: Occurrence) => void;
  /** Set in the week view, where a date opens its day. */
  onOpenDay?: (day: Date) => void;
  /** Set for editors. A click on an empty slot starts an event there. */
  onCreate?: (draft: EventDraft) => void;
}

/** A click lands on the half hour it falls in, and the new event runs an hour. */
const SLOT_MINUTES = 30;
const clock = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;

function timedDraft(day: Date, minutes: number): EventDraft {
  const start = Math.min(Math.floor(minutes / SLOT_MINUTES) * SLOT_MINUTES, DAY_MINUTES - SLOT_MINUTES);
  const end = start + 60;
  // An event made at 11:30 PM ends on the next day.
  const endDay = end >= DAY_MINUTES ? addDays(day, 1) : day;
  return { startsAt: `${dateKey(day)}T${clock(start)}`, endsAt: `${dateKey(endDay)}T${clock(end % DAY_MINUTES)}`, allDay: false };
}

/** Pixels per hour. The grid is 24 of these tall and scrolls. */
const HOUR = 48;
const DAY_MINUTES = 24 * 60;
/** A short event still needs room for its title, so it is drawn, and packed, as at least this long. */
const MIN_DRAWN_MINUTES = 30;

const hourLabels = Array.from({ length: 24 }, (_, hour) => `${hour % 12 === 0 ? 12 : hour % 12} ${hour < 12 ? 'AM' : 'PM'}`);

interface Segment {
  occurrence: Occurrence;
  start: number;
  end: number;
  column: number;
  columns: number;
}

/**
 * Side-by-side columns for events that overlap. Events are taken in start
 * order and each goes in the first column that is free; a run of events that
 * touch one another shares one column count, so they all get the same width.
 */
function pack(segments: Omit<Segment, 'column' | 'columns'>[]): Segment[] {
  const sorted = [...segments].sort((a, b) => a.start - b.start || b.end - a.end);
  const packed: Segment[] = [];
  let cluster: Segment[] = [];
  let columnEnds: number[] = [];
  let clusterEnd = -1;
  const flush = () => {
    for (const segment of cluster) segment.columns = columnEnds.length;
    packed.push(...cluster);
    cluster = [];
    columnEnds = [];
  };
  for (const segment of sorted) {
    const drawnEnd = Math.max(segment.end, segment.start + MIN_DRAWN_MINUTES);
    if (segment.start >= clusterEnd) flush();
    let column = columnEnds.findIndex((end) => end <= segment.start);
    if (column === -1) column = columnEnds.length;
    columnEnds[column] = drawnEnd;
    clusterEnd = Math.max(clusterEnd, drawnEnd);
    cluster.push({ ...segment, column, columns: 1 });
  }
  flush();
  return packed;
}

/** The day and week views: an all-day row over a 24-hour grid, one column per day. */
export function TimeGrid({ days, occurrences, today, onSelectEvent, onOpenDay, onCreate }: TimeGridProps) {
  const hover = useEventHover();
  const keys = useMemo(() => days.map(dateKey), [days]);
  const scroller = useRef<HTMLDivElement>(null);

  const { allDay, timed, earliest } = useMemo(() => {
    const allDayByKey = new Map<string, Occurrence[]>(keys.map((key) => [key, []]));
    const timedByKey = new Map<string, Omit<Segment, 'column' | 'columns'>[]>(keys.map((key) => [key, []]));
    let first = DAY_MINUTES;
    for (const occurrence of occurrences) {
      const start = torontoClock(new Date(occurrence.starts_at));
      // The end is exclusive: an event ending at midnight belongs to the day before.
      const end = torontoClock(new Date(new Date(occurrence.ends_at).getTime() - 1));
      for (const key of keys) {
        if (key < start.key || key > end.key) continue;
        if (occurrence.all_day) {
          allDayByKey.get(key)?.push(occurrence);
          continue;
        }
        // An event that crosses midnight is drawn once in each day it touches.
        const from = key === start.key ? start.minutes : 0;
        const to = key === end.key ? end.minutes + 1 : DAY_MINUTES;
        if (to <= from) continue;
        timedByKey.get(key)?.push({ occurrence, start: from, end: to });
        first = Math.min(first, from);
      }
    }
    return {
      allDay: allDayByKey,
      timed: new Map([...timedByKey].map(([key, segments]) => [key, pack(segments)])),
      earliest: first,
    };
  }, [keys, occurrences]);

  // Open on the working day rather than at midnight, unless something starts
  // earlier. It happens once for each range: events that load afterwards must
  // not yank the grid out from under the reader.
  const range = keys.join();
  const scrolledFor = useRef('');
  useLayoutEffect(() => {
    if (!scroller.current || scrolledFor.current === range) return;
    scrolledFor.current = range;
    scroller.current.scrollTop = (Math.min(7 * 60, Math.max(0, earliest - 30)) / 60) * HOUR;
  }, [range, earliest]);

  const [now, setNow] = useState(() => torontoClock(new Date()));
  useEffect(() => {
    const timer = window.setInterval(() => setNow(torontoClock(new Date())), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  const createAllDay = (index: number) => onCreate?.({ startsAt: `${keys[index]}T00:00`, endsAt: `${dateKey(addDays(days[index], 1))}T00:00`, allDay: true });

  // Editors always get the row, because it is where a click makes an all-day event.
  const hasAllDay = Boolean(onCreate) || [...allDay.values()].some((list) => list.length > 0);
  const columns = { gridTemplateColumns: `3.5rem repeat(${days.length}, minmax(0, 1fr))` };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* The header and the body are separate boxes, and only the body scrolls.
          Both reserve the scrollbar's width so their columns stay in line. */}
      <div className="grid shrink-0 overflow-hidden border-b border-ink/10 [scrollbar-gutter:stable] dark:border-line" style={columns}>
        <div />
        {days.map((day, index) => {
          const isToday = keys[index] === today;
          const number = <span className={`grid size-9 place-items-center rounded-full text-lg font-semibold ${isToday ? 'bg-deep text-white dark:bg-brand' : ''}`}>{day.getUTCDate()}</span>;
          return (
            <div key={keys[index]} className="flex flex-col items-center gap-0.5 border-l border-ink/10 py-2 dark:border-line">
              <span className={`text-xs font-bold uppercase tracking-wider ${isToday ? 'text-brand dark:text-link' : 'text-ink/45 dark:text-faint'}`}>{weekdayLabel(day)}</span>
              {onOpenDay
                ? <button type="button" onClick={() => onOpenDay(day)} aria-label={`Open ${keys[index]}`} className="rounded-full hover:bg-soft/30 dark:hover:bg-white/10">{number}</button>
                : number}
            </div>
          );
        })}
        {hasAllDay && (
          <>
            <div className="border-t border-ink/10 px-1 py-2 text-right text-[11px] text-ink/45 dark:border-line dark:text-faint">All day</div>
            {keys.map((key, index) => (
              <div key={key} className="relative min-h-8 min-w-0 border-l border-t border-ink/10 dark:border-line">
                {onCreate && (
                  <button
                    type="button"
                    aria-label={`Create an all-day event on ${dayLabel(key, true)}`}
                    onClick={() => createAllDay(index)}
                    className="absolute inset-0 hover:bg-soft/15 dark:hover:bg-white/[0.03]"
                  />
                )}
                {/* It takes the pointer so the wheel can scroll it; see the month
                    cell for why. The cap follows the window so a short one keeps
                    room for the hours. */}
                {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- the create button above is the keyboard path */}
                <div
                  className="relative max-h-[min(6rem,18dvh)] min-h-8 space-y-1 overflow-y-auto p-1"
                  onClick={(event) => { if (event.target === event.currentTarget) createAllDay(index); }}
                >
                  {(allDay.get(key) || []).map((occurrence) => (
                    <button key={`${occurrence.series_id}-${occurrence.recurrence_id_local}`} type="button" onClick={() => onSelectEvent(occurrence)} {...hover(occurrence)} className={`block w-full truncate rounded-md border-l-[3px] px-2 py-1 text-left text-xs font-semibold hover:brightness-95 dark:hover:brightness-110 ${eventTone(occurrence)}`}>
                      {occurrence.title}
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto [scrollbar-gutter:stable]">
        <div className="relative grid" style={{ ...columns, height: 24 * HOUR }}>
          <div className="relative">
            {hourLabels.map((label, hour) => hour > 0 && (
              <span key={label} className="absolute right-2 -translate-y-1/2 text-[11px] text-ink/45 dark:text-faint" style={{ top: hour * HOUR }}>{label}</span>
            ))}
          </div>
          {keys.map((key, index) => (
            <div
              key={key}
              className="relative border-l border-ink/10 dark:border-line"
              // The hour lines are one repeating background, not 24 elements per day.
              style={{ backgroundImage: 'linear-gradient(to bottom, color-mix(in srgb, currentColor 10%, transparent) 1px, transparent 1px)', backgroundSize: `100% ${HOUR}px` }}
            >
              {/* One create target under the whole day. The pointer's height
                  in it is the time. A key press has no height, so it starts at 9 AM. */}
              {onCreate && (
                <button
                  type="button"
                  aria-label={`Create an event on ${dayLabel(key, true)}`}
                  onClick={(event) => onCreate(timedDraft(days[index], event.detail === 0 ? 9 * 60 : (event.nativeEvent.offsetY / HOUR) * 60))}
                  className="absolute inset-0 cursor-cell"
                />
              )}
              {(timed.get(key) || []).map((segment) => {
                const drawn = Math.max(segment.end - segment.start, MIN_DRAWN_MINUTES);
                return (
                  <button
                    key={`${segment.occurrence.series_id}-${segment.occurrence.recurrence_id_local}`}
                    type="button"
                    onClick={() => onSelectEvent(segment.occurrence)}
                    {...hover(segment.occurrence)}
                    className={`absolute overflow-hidden rounded-md border-l-[3px] px-1.5 py-0.5 text-left hover:brightness-95 dark:hover:brightness-110 ${eventTone(segment.occurrence)}`}
                    style={{
                      top: (segment.start / 60) * HOUR + 1,
                      height: (drawn / 60) * HOUR - 2,
                      left: `calc(${(segment.column / segment.columns) * 100}% + 2px)`,
                      width: `calc(${100 / segment.columns}% - 4px)`,
                    }}
                  >
                    <span className="block truncate text-xs font-semibold">{segment.occurrence.title}</span>
                    {drawn >= 45 && <span className="block truncate text-[11px] text-ink/65 dark:text-muted">{timeLabel(segment.occurrence.starts_at, false)}</span>}
                  </button>
                );
              })}
              {key === now.key && (
                <div className="pointer-events-none absolute inset-x-0 z-10 border-t-2 border-red-500" style={{ top: (now.minutes / 60) * HOUR }} aria-hidden="true">
                  <span className="absolute -left-1 -top-[5px] size-2 rounded-full bg-red-500" />
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
