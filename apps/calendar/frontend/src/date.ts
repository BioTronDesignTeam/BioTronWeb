import type { Occurrence } from './types';

export const TORONTO_TIMEZONE = 'America/Toronto';

// Building an Intl formatter is expensive, and these are called once per event
// per render, so every one of them is created once at module scope.
const dateParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
});

const monthFormat = new Intl.DateTimeFormat('en-CA', { month: 'long', year: 'numeric', timeZone: 'UTC' });

const shortDayFormat = new Intl.DateTimeFormat('en-CA', {
  weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC',
});

const longDayFormat = new Intl.DateTimeFormat('en-CA', {
  weekday: 'long', month: 'long', day: 'numeric', timeZone: 'UTC',
});

const torontoTimeFormat = new Intl.DateTimeFormat('en-CA', {
  hour: 'numeric', minute: '2-digit', timeZone: TORONTO_TIMEZONE,
});

const torontoLongDayFormat = new Intl.DateTimeFormat('en-CA', {
  weekday: 'long', month: 'long', day: 'numeric', timeZone: TORONTO_TIMEZONE,
});

// A wall-clock string carries no offset, so it is read in a UTC frame and
// formatted there; anything else would re-apply a Toronto offset twice.
const wallClockTimeFormat = new Intl.DateTimeFormat('en-CA', {
  hour: 'numeric', minute: '2-digit', timeZone: 'UTC',
});

export function occurrenceDateKey(occurrence: Occurrence) {
  const parts = dateParts.formatToParts(new Date(occurrence.starts_at));
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}`;
}

export function dateKey(date: Date) {
  return date.toISOString().slice(0, 10);
}

export function dayLabel(key: string, long = false) {
  const date = new Date(`${key}T12:00:00Z`);
  return (long ? longDayFormat : shortDayFormat).format(date);
}

export function timeLabel(iso: string, allDay: boolean) {
  if (allDay) return 'All day';
  return torontoTimeFormat.format(new Date(iso));
}

/** 12-hour time from a stored wall-clock value, matching every other surface. */
export function timeOfDayLabel(localDateTime: string) {
  return wallClockTimeFormat.format(new Date(`${localDateTime.slice(0, 19)}Z`));
}

export function fullDateTimeLabel(occurrence: Occurrence) {
  if (occurrence.all_day) return `${dayLabel(occurrenceDateKey(occurrence), true)} · All day`;
  const start = new Date(occurrence.starts_at);
  const end = new Date(occurrence.ends_at);
  return `${torontoLongDayFormat.format(start)} · ${torontoTimeFormat.format(start)}–${torontoTimeFormat.format(end)} ET`;
}

export function addMonths(date: Date, amount: number) {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + amount, 1));
}

export function calendarRange(month: Date) {
  // Month state is already a UTC calendar anchor. Reinterpreting that instant in
  // Toronto would turn midnight UTC on the first into the previous local month.
  const first = new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth(), 1));
  const gridStart = new Date(first);
  gridStart.setUTCDate(first.getUTCDate() - first.getUTCDay());
  const nextMonth = addMonths(first, 1);
  const last = new Date(nextMonth);
  last.setUTCDate(0);
  const gridEnd = new Date(last);
  gridEnd.setUTCDate(last.getUTCDate() + (6 - last.getUTCDay()) + 1);
  return { from: dateKey(gridStart), to: dateKey(gridEnd), gridStart, gridEnd };
}

export function calendarDays(month: Date) {
  const { gridStart, gridEnd } = calendarRange(month);
  const days: Date[] = [];
  for (const day = new Date(gridStart); day < gridEnd; day.setUTCDate(day.getUTCDate() + 1)) {
    days.push(new Date(day));
  }
  return days;
}

export function localInput(value: string) {
  return value.slice(0, 16);
}

export function localDate(value?: string) {
  return value ? value.slice(0, 10) : '';
}

/** How much of the calendar is on screen at once. */
export type CalendarView = 'day' | 'week' | 'month';

const shortMonthFormat = new Intl.DateTimeFormat('en-CA', { month: 'short', timeZone: 'UTC' });
const shortMonthYearFormat = new Intl.DateTimeFormat('en-CA', { month: 'short', year: 'numeric', timeZone: 'UTC' });
const dayAnchorFormat = new Intl.DateTimeFormat('en-CA', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' });
const weekdayFormat = new Intl.DateTimeFormat('en-CA', { weekday: 'short', timeZone: 'UTC' });

const torontoClockFormat = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE, hourCycle: 'h23',
  year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
});

/**
 * Today in Toronto as a UTC calendar anchor. Every date the views pass around
 * is one of these: midnight UTC standing for a calendar day, never an instant.
 */
export function todayAnchor(now = new Date()) {
  const values = Object.fromEntries(dateParts.formatToParts(now).map((part) => [part.type, part.value]));
  return new Date(Date.UTC(Number(values.year), Number(values.month) - 1, Number(values.day)));
}

export function addDays(date: Date, amount: number) {
  const next = new Date(date);
  next.setUTCDate(next.getUTCDate() + amount);
  return next;
}

export function startOfWeek(date: Date) {
  return addDays(date, -date.getUTCDay());
}

/** The days a view draws: one, the Sunday-to-Saturday week, or the whole-week month grid. */
export function viewDays(view: CalendarView, anchor: Date) {
  if (view === 'month') return calendarDays(anchor);
  const first = view === 'week' ? startOfWeek(anchor) : anchor;
  return Array.from({ length: view === 'week' ? 7 : 1 }, (_, index) => addDays(first, index));
}

/** The API range for a view. `to` is the day after the last one drawn. */
export function viewRange(view: CalendarView, anchor: Date) {
  const days = viewDays(view, anchor);
  return { from: dateKey(days[0]), to: dateKey(addDays(days[days.length - 1], 1)) };
}

/** One step of the arrows: a day, a week, or a month. A month step lands on the first. */
export function stepAnchor(view: CalendarView, anchor: Date, direction: -1 | 1) {
  if (view === 'month') return addMonths(anchor, direction);
  return addDays(anchor, direction * (view === 'week' ? 7 : 1));
}

export function viewLabel(view: CalendarView, anchor: Date) {
  if (view === 'day') return dayAnchorFormat.format(anchor);
  if (view === 'month') return monthFormat.format(anchor);
  const first = startOfWeek(anchor);
  const last = addDays(first, 6);
  if (first.getUTCMonth() === last.getUTCMonth()) return monthFormat.format(first);
  if (first.getUTCFullYear() === last.getUTCFullYear()) return `${shortMonthFormat.format(first)} – ${shortMonthYearFormat.format(last)}`;
  return `${shortMonthYearFormat.format(first)} – ${shortMonthYearFormat.format(last)}`;
}

export function weekdayLabel(date: Date) {
  return weekdayFormat.format(date);
}

/** Where an instant falls on the Toronto wall clock: its calendar day and the minutes since that day's midnight. */
export function torontoClock(instant: Date) {
  const values = Object.fromEntries(torontoClockFormat.formatToParts(instant).map((part) => [part.type, part.value]));
  return { key: `${values.year}-${values.month}-${values.day}`, minutes: Number(values.hour) * 60 + Number(values.minute) };
}

