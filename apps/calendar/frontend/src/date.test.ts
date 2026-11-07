import { describe, expect, it } from 'vitest';
import {
  addDays,
  addLocalDays,
  addMonths,
  addWallClockHour,
  calendarRange,
  dateKey,
  keyDate,
  occurrenceDateKey,
  startOfWeek,
  stepAnchor,
  todayAnchor,
  torontoClock,
  torontoLocalInput,
  viewDays,
  viewRange,
} from './date';
import type { Occurrence } from './types';

const day = (key: string) => keyDate(key);
const keys = (dates: Date[]) => dates.map(dateKey);

describe('the test environment', () => {
  it('runs outside Toronto, so no helper leans on the machine time zone', () => {
    expect(new Date(2026, 0, 1).getTimezoneOffset()).toBe(-540);
  });
});

describe('keyDate and dateKey', () => {
  it('round-trip a calendar day as a UTC midnight anchor', () => {
    expect(keyDate('2026-11-07').toISOString()).toBe('2026-11-07T00:00:00.000Z');
    expect(dateKey(keyDate('2028-02-29'))).toBe('2028-02-29');
    expect(dateKey(keyDate('2026-11-07T15:30'))).toBe('2026-11-07');
  });
});

describe('addLocalDays', () => {
  it('moves across month and year ends', () => {
    expect(addLocalDays('2026-02-28', 1)).toBe('2026-03-01');
    expect(addLocalDays('2026-03-01', -1)).toBe('2026-02-28');
    expect(addLocalDays('2026-12-31', 1)).toBe('2027-01-01');
  });

  it('keeps the day across a daylight-saving change', () => {
    expect(addLocalDays('2026-03-07', 1)).toBe('2026-03-08');
    expect(addLocalDays('2026-11-01', 1)).toBe('2026-11-02');
  });

  it('reads only the date of a date-time value', () => {
    expect(addLocalDays('2026-11-07T23:30', 1)).toBe('2026-11-08');
  });
});

describe('addWallClockHour', () => {
  it('adds one wall-clock hour, even through the spring-forward gap', () => {
    expect(addWallClockHour('2026-03-08T01:30')).toBe('2026-03-08T02:30');
    expect(addWallClockHour('2026-11-01T01:30')).toBe('2026-11-01T02:30');
  });

  it('rolls into the next day and year', () => {
    expect(addWallClockHour('2026-12-31T23:15')).toBe('2027-01-01T00:15');
  });
});

describe('torontoLocalInput and torontoClock', () => {
  it('read an instant on the Toronto wall clock in both seasons', () => {
    expect(torontoLocalInput(new Date('2026-07-01T16:05:00Z'))).toBe('2026-07-01T12:05');
    expect(torontoLocalInput(new Date('2026-01-15T17:30:00Z'))).toBe('2026-01-15T12:30');
    expect(torontoClock(new Date('2026-07-01T16:05:00Z'))).toEqual({
      key: '2026-07-01',
      minutes: 12 * 60 + 5,
    });
  });

  it('use the 24-hour clock at midnight', () => {
    expect(torontoLocalInput(new Date('2026-01-16T05:00:00Z'))).toBe('2026-01-16T00:00');
    expect(torontoClock(new Date('2026-01-16T05:00:00Z'))).toEqual({
      key: '2026-01-16',
      minutes: 0,
    });
  });
});

describe('todayAnchor and occurrenceDateKey', () => {
  it('take the day from Toronto, not UTC', () => {
    // 03:00 UTC on the 8th is 22:00 on the 7th in Toronto.
    expect(dateKey(todayAnchor(new Date('2026-11-08T03:00:00Z')))).toBe('2026-11-07');
    expect(dateKey(todayAnchor(new Date('2026-11-08T05:00:00Z')))).toBe('2026-11-08');
    const occurrence = { starts_at: '2026-11-08T04:30:00Z' } as Occurrence;
    expect(occurrenceDateKey(occurrence)).toBe('2026-11-07');
  });
});

describe('weeks and months', () => {
  it('start a week on Sunday', () => {
    expect(dateKey(startOfWeek(day('2026-11-04')))).toBe('2026-11-01');
    expect(dateKey(startOfWeek(day('2026-11-01')))).toBe('2026-11-01');
    expect(dateKey(startOfWeek(day('2026-11-07')))).toBe('2026-11-01');
  });

  it('move a month to its first day', () => {
    expect(dateKey(addMonths(day('2026-01-31'), 1))).toBe('2026-02-01');
    expect(dateKey(addMonths(day('2026-01-15'), -1))).toBe('2025-12-01');
  });

  it('draw whole weeks around a month', () => {
    // November 2026 starts on a Sunday and ends on a Monday.
    const range = calendarRange(day('2026-11-15'));
    expect(range.from).toBe('2026-11-01');
    expect(range.to).toBe('2026-12-06');
    expect(viewDays('month', day('2026-11-15'))).toHaveLength(35);
    // February 2026 starts on a Sunday and ends on a Saturday: four rows.
    expect(viewDays('month', day('2026-02-10'))).toHaveLength(28);
  });
});

describe('viewDays and viewRange', () => {
  it('draw one day, a Sunday-to-Saturday week, or the month grid', () => {
    expect(keys(viewDays('day', day('2026-11-04')))).toEqual(['2026-11-04']);
    expect(keys(viewDays('week', day('2026-11-04')))).toEqual([
      '2026-11-01',
      '2026-11-02',
      '2026-11-03',
      '2026-11-04',
      '2026-11-05',
      '2026-11-06',
      '2026-11-07',
    ]);
  });

  it('end the API range the day after the last day drawn', () => {
    expect(viewRange('day', day('2026-11-04'))).toEqual({ from: '2026-11-04', to: '2026-11-05' });
    expect(viewRange('week', day('2026-12-30'))).toEqual({ from: '2026-12-27', to: '2027-01-03' });
    expect(viewRange('month', day('2026-11-15'))).toEqual({ from: '2026-11-01', to: '2026-12-06' });
  });
});

describe('stepAnchor', () => {
  it('steps by the view’s own unit', () => {
    expect(dateKey(stepAnchor('day', day('2026-11-07'), 1))).toBe('2026-11-08');
    expect(dateKey(stepAnchor('week', day('2026-11-07'), -1))).toBe('2026-10-31');
    expect(dateKey(stepAnchor('month', day('2026-11-07'), 1))).toBe('2026-12-01');
  });

  it('agrees with addDays across the autumn clock change', () => {
    expect(dateKey(addDays(day('2026-10-31'), 2))).toBe('2026-11-02');
  });
});
