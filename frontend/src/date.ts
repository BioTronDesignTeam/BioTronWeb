import type { Occurrence } from './types';

export const TORONTO_TIMEZONE = 'America/Toronto';

const dateParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: TORONTO_TIMEZONE,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
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
  return new Intl.DateTimeFormat('en-CA', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(month);
}

export function dayLabel(key: string, long = false) {
  const date = new Date(`${key}T12:00:00Z`);
  return new Intl.DateTimeFormat('en-CA', {
    weekday: long ? 'long' : 'short',
    month: long ? 'long' : 'short',
    day: 'numeric',
    timeZone: 'UTC',
  }).format(date);
}

export function timeLabel(iso: string, allDay: boolean) {
  if (allDay) return 'All day';
  return new Intl.DateTimeFormat('en-CA', {
    hour: 'numeric',
    minute: '2-digit',
    timeZone: TORONTO_TIMEZONE,
  }).format(new Date(iso));
}

export function fullDateTimeLabel(occurrence: Occurrence) {
  if (occurrence.all_day) return `${dayLabel(occurrenceDateKey(occurrence), true)} · All day`;
  const start = new Date(occurrence.starts_at);
  const end = new Date(occurrence.ends_at);
  const day = new Intl.DateTimeFormat('en-CA', {
    weekday: 'long', month: 'long', day: 'numeric', timeZone: TORONTO_TIMEZONE,
  }).format(start);
  const time = new Intl.DateTimeFormat('en-CA', {
    hour: 'numeric', minute: '2-digit', timeZone: TORONTO_TIMEZONE,
  });
  return `${day} · ${time.format(start)}–${time.format(end)} ET`;
}

export function startOfMonth(date = new Date()) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: TORONTO_TIMEZONE, year: 'numeric', month: '2-digit',
  }).formatToParts(date);
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
