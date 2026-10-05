import { describe, expect, it } from 'vitest';

import {
  cacheHitRate,
  formatTokens,
  formatUsd,
  planEndDate,
  projectName,
  quotaSlots,
  resetCountdown,
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

  // Cards in a grid share one size, so every card gets as many window rows as
  // the busiest one, the missing ones drawn empty.
  it('gives every plan card the same number of window rows, shortest window first', () => {
    const quota = (tool: string, names: string[]): AiUsageQuotaView => ({
      tool,
      deviceName: 'laptop',
      success: true,
      planLabel: '',
      activeUntil: '',
      error: '',
      queriedAt: 0,
      tiers: names.map((name) => ({ name, utilization: 10, resetsAt: '' })),
    });
    const slots = quotaSlots([
      quota('codex', ['seven_day']),
      quota('claude', ['seven_day_opus', 'five_hour', 'seven_day']),
    ]);
    expect(slots.map((s) => s.tool)).toEqual(['claude', 'codex']);
    expect(slots[0].tiers.map((t) => t?.name)).toEqual([
      'five_hour',
      'seven_day',
      'seven_day_opus',
    ]);
    expect(slots[1].tiers.map((t) => t?.name ?? null)).toEqual(['seven_day', null, null]);
  });

  it('keeps a card for a tool that has not reported yet', () => {
    const slots = quotaSlots([]);
    expect(slots.map((s) => [s.tool, s.quota, s.tiers.length])).toEqual([
      ['claude', null, 2],
      ['codex', null, 2],
    ]);
  });

  it("gives the plan's last day only when the reading has a real one", () => {
    // Midday UTC is the same calendar day in every time zone the panel runs in.
    expect(planEndDate('2026-10-30T12:00:00+00:00')).toBe('2026-10-30');
    expect(planEndDate('')).toBeNull();
    expect(planEndDate('soon')).toBeNull();
  });
});
