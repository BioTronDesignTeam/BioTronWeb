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

const monthAnchorFormat = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE, year: 'numeric', month: '2-digit',
});

export function occurrenceDateKey(occurrence: Occurrence) {
  const parts = dateParts.formatToParts(new Date(occurrence.starts_at));
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}`;
}

export function dateKey(date: Date) {
  return date.toISOString().slice(0, 10);
}

export function monthLabel(month: Date) {
  return monthFormat.format(month);
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

export function startOfMonth(date = new Date()) {
  const parts = monthAnchorFormat.formatToParts(date);
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return new Date(Date.UTC(Number(values.year), Number(values.month) - 1, 1));
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
