import { describe, expect, it } from 'vitest';

import { daysUntilReset } from '@/lib/traffic/resetDay';

const on = (y: number, m: number, d: number) => new Date(y, m - 1, d, 15, 30);

describe("days until Lite's monthly reset", () => {
  it.each([
    ['later this month', 22, on(2026, 10, 2), 20],
    ['today', 2, on(2026, 10, 2), 0],
    ['next month once it has passed', 1, on(2026, 10, 2), 30],
    ['the last day of a month that has it', 30, on(2026, 11, 20), 10],
    ['a day the month lacks, on the 1st after it', 31, on(2026, 11, 20), 11],
    ["last month's rolled-over day, today", 31, on(2026, 3, 1), 0],
  ])('counts %s', (_name, day, now, want) => {
    expect(daysUntilReset(day, now)).toBe(want);
  });

  it('has nothing to count without a reset day', () => {
    expect(daysUntilReset(0, on(2026, 10, 2))).toBeNull();
    expect(daysUntilReset(32, on(2026, 10, 2))).toBeNull();
  });
});
