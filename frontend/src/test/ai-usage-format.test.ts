import { describe, expect, it } from 'vitest';

import {
  cacheHitRate,
  formatTokens,
  formatUsd,
  planEndDate,
  periodDays,
  projectName,
  resetCountdown,
  windowsForTool,
} from '@/pages/ai-usage/aiUsageFormat';
import type { AiUsageQuotaView } from '@/generated/zod';

describe('aiUsageFormat', () => {
  it('shortens token counts at each power of a thousand', () => {
    expect(formatTokens(999)).toBe('999');
    expect(formatTokens(12_345)).toBe('12.3K');
    expect(formatTokens(3_200_000)).toBe('3.2M');
    expect(formatTokens(6_190_000_000)).toBe('6.19B');
  });

  it('writes dollars with cents, and says when a cost is under a cent', () => {
    expect(formatUsd(1234.5)).toBe('$1,234.50');
    expect(formatUsd(0)).toBe('$0.00');
    expect(formatUsd(0.004)).toBe('<$0.01');
  });

  // A session started in a folder, on Linux, macOS or Windows.
  it('names a project by its last folder', () => {
    expect(projectName('/home/dev/code/app')).toBe('app');
    expect(projectName('/home/dev/code/app/')).toBe('app');
    expect(projectName('C:\\Users\\dev\\repo')).toBe('repo');
    expect(projectName('')).toBe('');
  });

  it('measures cache hits against every input token, and has none without input', () => {
    expect(
      cacheHitRate({ inputTokens: 100, cacheReadTokens: 800, cacheWriteTokens: 100 }),
    ).toBeCloseTo(0.8);
    expect(cacheHitRate({ inputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 })).toBeNull();
  });

  it('counts down to a reset and drops one that has passed', () => {
    const now = Date.parse('2026-10-06T10:00:00Z');
    expect(resetCountdown('2026-10-06T13:05:00Z', now)).toBe('3h 5m');
    expect(resetCountdown('2026-10-08T14:00:00Z', now)).toBe('2d 4h');
    expect(resetCountdown('2026-10-06T10:12:30Z', now)).toBe('12m');
    expect(resetCountdown('2026-10-06T09:00:00Z', now)).toBeNull();
    expect(resetCountdown('', now)).toBeNull();
  });

  // One tool's windows, shortest first: the estimates Pigger Switch sent, each with the
  // plan's own reading beside it; an older Pigger Switch sends none, so the bare readings.
  it("lists a tool's windows from its estimates, or its readings without them", () => {
    const quota = (estimates: AiUsageQuotaView['estimates']): AiUsageQuotaView => ({
      tool: 'claude',
      deviceName: 'laptop',
      success: true,
      planLabel: 'Pro',
      activeUntil: '',
      error: '',
      queriedAt: 0,
      tiers: ['seven_day', 'five_hour'].map((name) => ({ name, utilization: 10, resetsAt: '' })),
      estimates,
    });
    const window = (tier: string) => ({
      tier,
      used: { requests: 1, costUsd: 1, totalTokens: 1 },
    });
    const withEstimates = windowsForTool(
      quota({
        windows: [window('seven_day_opus'), window('seven_day'), window('five_hour')],
        fiveHourHistory: [],
        weeklyHistory: [],
      }),
    );
    expect(withEstimates.map((w) => [w.tier, w.estimate?.tier, w.reading?.name ?? null])).toEqual([
      ['five_hour', 'five_hour', 'five_hour'],
      ['seven_day', 'seven_day', 'seven_day'],
      ['seven_day_opus', 'seven_day_opus', null],
    ]);
    const bare = windowsForTool(quota(null));
    expect(bare.map((w) => [w.tier, w.estimate])).toEqual([
      ['five_hour', null],
      ['seven_day', null],
    ]);
    expect(windowsForTool({ ...quota(null), success: false })).toEqual([]);
    expect(windowsForTool(undefined)).toEqual([]);
  });

  it('counts the days a period covers so far, at least one', () => {
    expect(periodDays('month', '2026-10-01', undefined, '2026-10-06')).toBe(6);
    expect(periodDays('week', '2026-10-05', undefined, '2026-10-06')).toBe(2);
    expect(periodDays('today', '2026-10-06', undefined, '2026-10-06')).toBe(1);
    expect(periodDays('all', '', '2026-09-24', '2026-10-06')).toBe(13);
    expect(periodDays('all', '', undefined, '2026-10-06')).toBe(1);
  });

  it("gives the plan's last day only when the reading has a real one", () => {
    // Midday UTC is the same calendar day in every time zone the panel runs in.
    expect(planEndDate('2026-10-30T12:00:00+00:00')).toBe('2026-10-30');
    expect(planEndDate('')).toBeNull();
    expect(planEndDate('soon')).toBeNull();
  });
});
