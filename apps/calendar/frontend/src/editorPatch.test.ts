import { describe, expect, it } from 'vitest';
import { torontoLocalInput } from './date';
import {
  eventPayload,
  occurrencePatch,
  seriesBaseline,
  toAllDay,
  toTimed,
  type PatchFields,
  type Schedule,
} from './editorPatch';
import type { EventSeries, Occurrence } from './types';

// A weekly 18:00-19:30 meeting, Toronto wall clock, through the DST change on
// Sunday 1 November 2026.
const series: EventSeries = {
  id: 's1',
  uid: 'u1',
  scope_id: 't',
  scope_name: 'BioTron',
  scope_path: 'BioTron',
  scope_kind: 'TEAM',
  state: 'PUBLISHED',
  title: 'General meeting',
  description: 'Weekly sync.',
  location: 'E5 2004',
  url: '',
  starts_at_local: '2026-10-22T18:00:00',
  ends_at_local: '2026-10-22T19:30:00',
  timezone: 'America/Toronto',
  all_day: false,
  recurrence_until: '2026-12-10',
  sequence: 3,
};

// The occurrence the API sends for one week: UTC instants for that week's
// wall clock (EDT is UTC-4 before 1 November, EST is UTC-5 after it).
function occurrence(recurrenceId: string, startsAt: string, endsAt: string): Occurrence {
  return {
    series_id: series.id,
    uid: series.uid,
    scope_id: series.scope_id,
    scope_name: series.scope_name,
    scope_path: series.scope_path,
    scope_kind: series.scope_kind,
    title: series.title,
    description: series.description,
    location: series.location,
    url: series.url,
    starts_at: startsAt,
    ends_at: endsAt,
    all_day: false,
    timezone: series.timezone,
    recurrence_id_local: recurrenceId,
    recurring: true,
    modified: false,
    series_sequence: series.sequence,
  };
}

const beforeDST = occurrence('2026-10-29T18:00:00', '2026-10-29T22:00:00Z', '2026-10-29T23:30:00Z');
const afterDST = occurrence('2026-11-05T18:00:00', '2026-11-05T23:00:00Z', '2026-11-06T00:30:00Z');

// What OccurrenceEditor holds for an occurrence nobody has touched yet.
function untouched(item: Occurrence): PatchFields {
  return {
    title: item.title,
    description: item.description,
    location: item.location,
    url: item.url,
    starts_at_local: `${torontoLocalInput(new Date(item.starts_at))}:00`,
    ends_at_local: `${torontoLocalInput(new Date(item.ends_at))}:00`,
  };
}

describe('occurrence patch', () => {
  it('is the series at the generated start', () => {
    expect(seriesBaseline(afterDST, series)).toMatchObject({
      starts_at_local: '2026-11-05T18:00:00',
      ends_at_local: '2026-11-05T19:30:00',
    });
  });

  it('is empty for an untouched submit, on either side of the DST change', () => {
    for (const item of [beforeDST, afterDST]) {
      expect(occurrencePatch(untouched(item), seriesBaseline(item, series))).toEqual({});
    }
  });

  it('holds only the title when only the title changed', () => {
    const edited = { ...untouched(afterDST), title: 'General meeting (moved online)' };
    expect(occurrencePatch(edited, seriesBaseline(afterDST, series))).toEqual({
      title: 'General meeting (moved online)',
    });
  });

  it('holds both times when the occurrence moved', () => {
    const edited = {
      ...untouched(beforeDST),
      starts_at_local: '2026-10-29T19:00:00',
      ends_at_local: '2026-10-29T20:30:00',
    };
    expect(occurrencePatch(edited, seriesBaseline(beforeDST, series))).toEqual({
      starts_at_local: '2026-10-29T19:00:00',
      ends_at_local: '2026-10-29T20:30:00',
    });
  });
});

const schedule = (changes: Partial<Schedule> = {}): Schedule => ({
  startsAt: '2026-11-04T18:00',
  endsAt: '2026-11-04T19:00',
  allDay: false,
  weekly: false,
  recurrenceUntil: '',
  ...changes,
});

describe('series payload', () => {
  const fields = { scope_id: 't', title: 'Build night', description: '', location: '', url: '' };

  it('sends seconds, no repeat, and sequence 0 for a new one-off event', () => {
    expect(eventPayload(fields, schedule(), undefined)).toEqual({
      ...fields,
      starts_at_local: '2026-11-04T18:00:00',
      ends_at_local: '2026-11-04T19:00:00',
      all_day: false,
      recurrence_until: '',
      expected_sequence: 0,
    });
  });

  it('sends the inclusive repeat end and the sequence it was read at', () => {
    const body = eventPayload(
      fields,
      schedule({ weekly: true, recurrenceUntil: '2026-12-09' }),
      series.sequence,
    );
    expect(body).toMatchObject({ recurrence_until: '2026-12-09', expected_sequence: 3 });
  });

  it('drops the repeat end once weekly is turned off', () => {
    const body = eventPayload(fields, schedule({ recurrenceUntil: '2026-12-09' }), 3);
    expect(body.recurrence_until).toBe('');
  });
});

describe('all-day switch', () => {
  it('makes a one-day event midnight to the next midnight', () => {
    expect(toAllDay(schedule())).toEqual({
      startsAt: '2026-11-04T00:00',
      endsAt: '2026-11-05T00:00',
    });
  });

  it('keeps the next day exclusive across the end of a month', () => {
    expect(
      toAllDay(schedule({ startsAt: '2026-11-30T18:00', endsAt: '2026-11-30T19:00' })),
    ).toEqual({ startsAt: '2026-11-30T00:00', endsAt: '2026-12-01T00:00' });
  });

  it('never ends before it starts', () => {
    expect(toAllDay(schedule({ endsAt: '2026-11-03T10:00' })).endsAt).toBe('2026-11-05T00:00');
  });

  it('gives the remembered times back, or an hour from 18:00', () => {
    const remembered = { startsAt: '2026-11-04T09:30', endsAt: '2026-11-04T11:00' };
    expect(toTimed(schedule(), remembered)).toBe(remembered);
    expect(toTimed(schedule({ startsAt: '2026-11-04T00:00' }))).toEqual({
      startsAt: '2026-11-04T18:00',
      endsAt: '2026-11-04T19:00',
    });
  });
});
