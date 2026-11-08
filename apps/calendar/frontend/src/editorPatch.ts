import { addWallClockHour } from './date';
import type { EventPayload, EventSeries, Occurrence } from './types';

/**
 * What the two editors send. The series editor sends a whole payload; the
 * occurrence editor sends only the fields that differ from the series. Both
 * are pure, kept out of the components so they can be tested on their own.
 */

export interface Schedule {
  /** `YYYY-MM-DDTHH:MM` wall clock, as a datetime-local input holds it. */
  startsAt: string;
  endsAt: string;
  allDay: boolean;
  weekly: boolean;
  recurrenceUntil: string;
}

/** All-day boundaries are midnight to midnight, with an exclusive end date. */
export function toAllDay({ startsAt, endsAt }: Pick<Schedule, 'startsAt' | 'endsAt'>) {
  const startDate = startsAt.slice(0, 10);
  const endDate = endsAt.slice(0, 10) <= startDate ? startDate : endsAt.slice(0, 10);
  const exclusive = new Date(`${endDate}T12:00:00Z`);
  if (endDate === startDate) exclusive.setUTCDate(exclusive.getUTCDate() + 1);
  return {
    startsAt: `${startDate}T00:00`,
    endsAt: `${exclusive.toISOString().slice(0, 10)}T00:00`,
  };
}

/** Restore the times the event had before it was made all-day, or a sensible hour. */
export function toTimed(
  { startsAt }: Pick<Schedule, 'startsAt'>,
  remembered?: { startsAt: string; endsAt: string },
) {
  if (remembered) return remembered;
  const start = `${startsAt.slice(0, 10)}T18:00`;
  return { startsAt: start, endsAt: addWallClockHour(start) };
}

/** The series editor's request body. */
export function eventPayload(
  fields: Pick<EventPayload, 'scope_id' | 'title' | 'description' | 'location' | 'url'>,
  schedule: Schedule,
  sequence: number | undefined,
): EventPayload {
  return {
    ...fields,
    starts_at_local: `${schedule.startsAt}:00`,
    ends_at_local: `${schedule.endsAt}:00`,
    all_day: schedule.allDay,
    // An empty string ends the repeat: the server reads it as "not weekly".
    recurrence_until: schedule.weekly ? schedule.recurrenceUntil : '',
    // A new event has no sequence yet; the server expects 0 for it.
    expected_sequence: sequence || 0,
  };
}

// Wall-clock arithmetic, done in a UTC frame so no daylight-saving change can
// shift the result. The series stores wall clock and so does the patch.
function wallClock(local: string) {
  return new Date(`${local.slice(0, 19)}Z`);
}

function addWallClockMs(local: string, milliseconds: number) {
  return new Date(wallClock(local).getTime() + milliseconds).toISOString().slice(0, 19);
}

/**
 * What this occurrence would be with no override at all: the series values, at
 * the generated start its recurrence id names. Anything equal to this is not a
 * change and must stay out of the patch, or the occurrence silently stops
 * following later series-wide edits.
 */
export function seriesBaseline(occurrence: Occurrence, series: EventSeries) {
  const duration =
    wallClock(series.ends_at_local).getTime() - wallClock(series.starts_at_local).getTime();
  const start = occurrence.recurrence_id_local.slice(0, 19);
  return {
    title: series.title,
    description: series.description,
    location: series.location,
    url: series.url,
    starts_at_local: start,
    ends_at_local: addWallClockMs(start, duration),
  };
}

export type PatchFields = ReturnType<typeof seriesBaseline>;

/** Only the fields the editor changed from the series, so the rest keep following it. */
export function occurrencePatch(
  edited: PatchFields,
  baseline: PatchFields,
): Record<string, string> {
  const patch: Record<string, string> = {};
  for (const field of Object.keys(edited) as (keyof PatchFields)[]) {
    if (edited[field] !== baseline[field]) patch[field] = edited[field];
  }
  return patch;
}
