import { describe, expect, it } from 'vitest';
import { readGuardForm, SUBJECT_LABELS, SUBJECTS, textToIds } from './guardForm';

describe('readGuardForm', () => {
  it('refuses a guard with no role, and says how to turn the feature off', () => {
    for (const roles of ['', '  ', ', ,']) {
      expect(readGuardForm('123', roles, '')).toEqual({
        error: 'Add at least one role id. To turn this feature off, clear its guard.',
      });
    }
  });

  it('refuses a guild id that is not digits', () => {
    expect(readGuardForm('abc', '1', '')).toEqual({ error: 'Guild id must be a digit string.' });
  });

  it('refuses role and channel ids that are not digits', () => {
    expect(readGuardForm('1', '2, x', '')).toMatchObject({
      error: expect.stringContaining('Role'),
    });
    expect(readGuardForm('1', '2', '3, y')).toMatchObject({
      error: expect.stringContaining('Channel'),
    });
  });

  it('trims the ids and allows an empty channel list, which means any channel', () => {
    expect(readGuardForm(' 123 ', ' 4, 5 ,', '')).toEqual({
      body: { guild_id: '123', role_ids: ['4', '5'], channel_ids: [] },
    });
  });
});

describe('guard subjects', () => {
  it('have a label for every subject', () => {
    for (const subject of SUBJECTS) expect(SUBJECT_LABELS[subject]).toBeTruthy();
  });

  it('splits a comma list and drops empty parts', () => {
    expect(textToIds('1,,2 , ')).toEqual(['1', '2']);
  });
});
