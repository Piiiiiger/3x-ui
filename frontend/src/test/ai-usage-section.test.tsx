import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import AiUsageSection from '@/pages/ai-usage/AiUsageSection';
import type { AiUsageOverview, AiUsageQuotaView } from '@/generated/zod';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from '@/test/test-utils';

// jsdom has no canvas for the chart, and these tests are about the cards around it.
vi.mock('@/pages/ai-usage/AiDailyCard', () => ({ default: () => null }));

const NOW = Math.floor(new Date(2026, 9, 6, 10).getTime() / 1000);

const used = (costUsd: number, requests = 10) => ({ requests, costUsd, totalTokens: 1_000_000 });
const limitOf = (costUsd: number, basis: 'current' | 'typical' = 'current', windows = 1) => ({
  costUsd,
  costLow: costUsd - 5,
  costHigh: costUsd + 5,
  tokens: 570_000_000,
  basis,
  windows,
});

/** Claude as a current Pigger Switch reports it: every window measured, with past windows. */
function claudeQuota(): AiUsageQuotaView {
  return {
    tool: 'claude',
    deviceName: 'laptop',
    success: true,
    planLabel: 'Pro',
    activeUntil: '',
    error: '',
    queriedAt: (NOW - 120) * 1000,
    tiers: [
      { name: 'seven_day', utilization: 28, resetsAt: '' },
      { name: 'five_hour', utilization: 16, resetsAt: '' },
      { name: 'seven_day_fable', utilization: 4, resetsAt: '' },
    ],
    estimates: {
      windows: [
        {
          tier: 'seven_day',
          start: NOW - 2 * 86400,
          end: NOW + 5 * 86400,
          reportedUtilization: 28,
          used: used(376.38),
          limit: limitOf(1339.39, 'typical', 3),
          remainingCostUsd: 963,
          remainingTokens: 3_270_000_000,
          exhaustsAt: NOW + 3 * 86400,
          fiveHourWindowsLeft: 27,
          perFiveHourCostUsd: 35.67,
          perFiveHourTokens: 121_000_000,
        },
        {
          tier: 'five_hour',
          start: NOW - 3600,
          end: NOW + 4 * 3600,
          reportedUtilization: 16,
          estimatedUtilization: 18,
          used: used(29.62, 164),
          limit: limitOf(185.15),
          remainingCostUsd: 155.53,
          remainingTokens: 479_000_000,
          projectedUtilization: 67,
        },
        // The plan says 4%, but this computer sent nothing to Fable: used elsewhere.
        {
          tier: 'seven_day_fable',
          start: NOW - 2 * 86400,
          end: NOW + 5 * 86400,
          reportedUtilization: 4,
          used: used(0, 0),
        },
      ],
      fiveHourHistory: [
        {
          start: NOW - 3600,
          end: NOW + 4 * 3600,
          exact: true,
          current: true,
          used: used(29.62),
          peakUtilization: 16,
          limit: limitOf(185.15),
        },
        {
          start: NOW - 30 * 3600,
          end: NOW - 25 * 3600,
          exact: true,
          current: false,
          used: used(10),
          peakUtilization: 50,
          limit: limitOf(20),
        },
        {
          start: NOW - 40 * 3600,
          end: NOW - 35 * 3600,
          exact: true,
          current: false,
          used: used(15),
          peakUtilization: 50,
          limit: limitOf(30),
        },
        // No reading: $10 against the median limit ($30 of $20, $30, $185.15) is about 33%.
        {
          start: NOW - 50 * 3600,
          end: NOW - 45 * 3600,
          exact: false,
          current: false,
          used: used(10),
        },
      ],
      weeklyHistory: [
        {
          start: NOW - 9 * 86400,
          end: NOW - 2 * 86400,
          exact: true,
          current: false,
          used: used(140),
          peakUtilization: 70,
        },
      ],
    },
  };
}

/** Codex from an older Pigger Switch: the plan's percentages, no estimates. */
function codexQuota(): AiUsageQuotaView {
  return {
    tool: 'codex',
    deviceName: 'laptop',
    success: true,
    planLabel: 'Team',
    activeUntil: '2026-10-30T12:00:00+00:00',
    error: '',
    queriedAt: (NOW - 60) * 1000,
    tiers: [{ name: 'seven_day', utilization: 100, resetsAt: '' }],
    estimates: null,
  };
}

function overview(over: Partial<AiUsageOverview> = {}): AiUsageOverview {
  return {
    period: 'month',
    periodStart: '2026-10-01',
    totals: {
      costUsd: 13,
      claudeCostUsd: 11,
      codexCostUsd: 2,
      requests: 24,
      inputTokens: 240,
      outputTokens: 2400,
      cacheReadTokens: 24_000,
      cacheWriteTokens: 24,
      sessions: 2,
    },
    daily: [],
    projects: [
      {
        name: '/w/app',
        app: 'claude',
        costUsd: 11,
        claudeCostUsd: 11,
        codexCostUsd: 0,
        requests: 16,
        tokens: 1760,
      },
      {
        name: '/w/lib',
        app: 'codex',
        costUsd: 2,
        claudeCostUsd: 0,
        codexCostUsd: 2,
        requests: 8,
        tokens: 880,
      },
    ],
    models: [
      {
        name: 'opus',
        app: 'claude',
        costUsd: 10,
        claudeCostUsd: 10,
        codexCostUsd: 0,
        requests: 12,
        tokens: 1320,
      },
    ],
    sessions: [
      {
        app: 'claude',
        sessionId: 's-app',
        title: 'fix login',
        project: '/w/app',
        model: 'opus',
        deviceName: 'laptop',
        requests: 14,
        tokens: 1540,
        costUsd: 7,
        firstAt: 1,
        lastAt: 2,
      },
    ],
    quotas: [claudeQuota(), codexQuota()],
    devices: [
      {
        id: 1,
        name: 'laptop',
        appVersion: '1.0.0',
        lastSyncAt: 1,
        firstDay: '2026-09-20',
        lastDay: '2026-10-06',
        costUsd: 59,
      },
    ],
    ...over,
  };
}

let current: AiUsageOverview;
let getSpy: ReturnType<typeof vi.spyOn>;
let postSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  // The page remembers the chosen tool; every test starts from Claude Code.
  localStorage.clear();
  // Only Date is faked: the month so far is October 1–6, six days.
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(NOW * 1000);
  current = overview();
  getSpy = vi.spyOn(HttpUtil, 'get').mockImplementation(async (url, params) => {
    if (url === '/panel/api/aiUsage/overview') {
      const period = (params as { period: AiUsageOverview['period'] }).period;
      return new Msg(true, '', { ...current, period });
    }
    return new Msg(false, 'unexpected ' + url);
  });
  postSpy = vi.spyOn(HttpUtil, 'post').mockImplementation(async (url) => {
    if (url === '/panel/api/setting/apiTokens/create') {
      return new Msg(true, '', { token: 'fresh-upload-token' });
    }
    return new Msg(false, 'unexpected ' + url);
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// Table headers reuse some tile labels, so the lookup stays inside the tiles.
function tile(label: string) {
  const title = screen.getAllByText(label).find((el) => el.closest('.ov-tile'));
  return title?.closest('.ov-tile') as HTMLElement;
}

describe('AiUsageSection', () => {
  it('shows one tool at a time, Claude Code first, with what it cost a day', async () => {
    renderWithProviders(<AiUsageSection isMobile={false} />);

    await waitFor(() => expect(within(tile('Cost')).getByText('$13.00')).toBeTruthy());
    // $13 over October 1–6
    expect(within(tile('Cost')).getByText('$2.17 a day')).toBeTruthy();
    expect(within(tile('Sessions')).getByText('Across 2 projects')).toBeTruthy();
    // 24 cache reads of 24,264 input tokens.
    expect(within(tile('Tokens')).getByText('Cache hits 98.9%')).toBeTruthy();
    expect(getSpy).toHaveBeenCalledWith(
      '/panel/api/aiUsage/overview',
      { period: 'month', deviceId: 0, app: 'claude' },
      { silent: true },
    );
    const tools = screen.getByRole('radiogroup', { name: 'Tool' });
    expect(
      within(tools)
        .getAllByRole('radio')
        .map((r) => r.closest('label')?.textContent),
    ).toEqual(['Claude Code', 'Codex']);
  });

  it("gives each of the tool's windows its usage, limit and what is left, shortest first", async () => {
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Pro');

    const cards = Array.from(document.querySelectorAll<HTMLElement>('.ai-quota-card'));
    expect(cards.map((c) => c.querySelector('.ov-card-title')?.textContent)).toEqual([
      '5-hour window',
      'Weekly',
      'Weekly · Fable',
    ]);
    // Every card draws the same rows, so the grid keeps one size.
    expect(cards.map((c) => c.querySelectorAll('dt').length)).toEqual([4, 4, 4]);
    const [fiveHour, weekly, fable] = cards;
    expect(within(fiveHour).getByText(/^≈ \$185\.15/)).toBeTruthy();
    expect(within(fiveHour).getByText(/^≈ \$155\.53/)).toBeTruthy();
    expect(within(fiveHour).getByText('≈ 18% now')).toBeTruthy();
    expect(within(fiveHour).getByText('At this pace: about 67% by the reset')).toBeTruthy();
    expect(within(weekly).getByText(/^recent windows \(3\)/)).toBeTruthy();
    expect(within(weekly).getByText(/per 5 hours, 27 left$/)).toBeTruthy();
    expect(within(weekly).getByText(/^At this pace it runs out around/)).toBeTruthy();
    expect(
      within(fable).getByText('Used elsewhere (no requests on that computer), so no estimate'),
    ).toBeTruthy();
  });

  it("switches to Codex, whose older Pigger Switch sent only the plan's percentages", async () => {
    const user = userEvent.setup({ advanceTimers: () => {} });
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Pro');

    await user.click(screen.getByText('Codex', { selector: '.ant-segmented-item-label' }));
    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith(
        '/panel/api/aiUsage/overview',
        { period: 'month', deviceId: 0, app: 'codex' },
        { silent: true },
      ),
    );
    await screen.findByText('Team');
    const [card] = Array.from(document.querySelectorAll<HTMLElement>('.ai-quota-card'));
    expect(within(card).getByText('100%')).toBeTruthy();
    expect(
      within(card).getByText('Update Pigger Switch to see this window in dollars'),
    ).toBeTruthy();
    expect(document.querySelector('.ai-plan-head')?.textContent).toMatch(
      /Read on laptop · .* · Active until 2026-10-30/,
    );
  });

  it('opens on the tool chosen last time', async () => {
    localStorage.setItem('pigger-ai-usage-tool', 'codex');
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Team');
    expect(getSpy).toHaveBeenCalledWith(
      '/panel/api/aiUsage/overview',
      { period: 'month', deviceId: 0, app: 'codex' },
      { silent: true },
    );
  });

  it('says why there is no plan to show when the reading failed', async () => {
    current = overview({
      quotas: [
        { ...claudeQuota(), success: false, error: 'token expired', tiers: [], estimates: null },
      ],
    });
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Could not read the limits: token expired');
    expect(document.querySelector('.ai-quota-card')).toBeNull();
  });

  it("lists the tool's past windows and switches between 5-hour and weekly", async () => {
    const user = userEvent.setup({ advanceTimers: () => {} });
    renderWithProviders(<AiUsageSection isMobile={false} />);
    const title = await screen.findByText('Past windows');
    const card = title.closest('.ant-card') as HTMLElement;
    const rows = within(card).getAllByRole('row').slice(1);
    expect(rows).toHaveLength(4);
    expect(within(rows[0]).getByText('Now')).toBeTruthy();
    expect(within(rows[1]).getByText('50%')).toBeTruthy();
    expect(within(rows[3]).getByText('*')).toBeTruthy();
    expect(within(rows[3]).getByText('≈ 33%')).toBeTruthy();

    await user.click(within(card).getByText('Weekly windows'));
    const weekly = within(card).getAllByRole('row').slice(1);
    expect(weekly).toHaveLength(1);
    expect(within(weekly[0]).getByText('70%')).toBeTruthy();
    expect(within(weekly[0]).getByText('$140.00')).toBeTruthy();
  });

  it('asks for the chosen period and says where it starts', async () => {
    const user = userEvent.setup({ advanceTimers: () => {} });
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Since 2026-10-01');

    await user.click(screen.getByText('This week'));
    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith(
        '/panel/api/aiUsage/overview',
        { period: 'week', deviceId: 0, app: 'claude' },
        { silent: true },
      ),
    );
  });

  it('shows how to connect a computer while nothing has reported', async () => {
    current = overview({ devices: [], quotas: [], sessions: [], projects: [], models: [] });
    renderWithProviders(<AiUsageSection isMobile={false} />);

    await screen.findByText('No usage yet');
    expect(screen.getByRole('button', { name: /Connect a computer/ })).toBeTruthy();
    expect(document.querySelector('.ov-tile')).toBeNull();
  });

  it('mints an upload-only token for a computer and shows it with the panel URL', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await user.click(await screen.findByRole('button', { name: /Connect a computer/ }));

    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByPlaceholderText('e.g. laptop'), 'laptop');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await within(dialog).findByText('fresh-upload-token');
    expect(postSpy).toHaveBeenCalledWith('/panel/api/setting/apiTokens/create', {
      name: 'ai-usage-laptop',
      scope: 'ai-usage',
    });
    expect(within(dialog).getByText(window.location.origin)).toBeTruthy();
  });
});
