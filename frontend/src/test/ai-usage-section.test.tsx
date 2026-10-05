import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import AiUsageSection from '@/pages/ai-usage/AiUsageSection';
import type { AiUsageOverview } from '@/generated/zod';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from '@/test/test-utils';

// jsdom has no canvas for the chart, and these tests are about the cards around it.
vi.mock('@/pages/ai-usage/AiDailyCard', () => ({ default: () => null }));

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
    quotas: [
      {
        tool: 'claude',
        deviceName: 'laptop',
        success: true,
        planLabel: 'Max 5x',
        activeUntil: '',
        error: '',
        queriedAt: 0,
        tiers: [
          { name: 'seven_day', utilization: 21, resetsAt: '' },
          { name: 'five_hour', utilization: 45, resetsAt: '' },
          { name: 'seven_day_fable', utilization: 4, resetsAt: '' },
        ],
      },
      {
        tool: 'codex',
        deviceName: 'laptop',
        success: true,
        planLabel: 'Team',
        activeUntil: '',
        error: '',
        queriedAt: 0,
        tiers: [{ name: 'seven_day', utilization: 100, resetsAt: '' }],
      },
    ],
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
  vi.restoreAllMocks();
});

// Table headers reuse some tile labels, so the lookup stays inside the tiles.
function tile(label: string) {
  const title = screen.getAllByText(label).find((el) => el.closest('.ov-tile'));
  return title?.closest('.ov-tile') as HTMLElement;
}

describe('AiUsageSection', () => {
  it("fills the tiles with the period's cost, split by app, and its sessions", async () => {
    renderWithProviders(<AiUsageSection isMobile={false} />);

    await waitFor(() => expect(within(tile('Cost')).getByText('$13.00')).toBeTruthy());
    expect(within(tile('Cost')).getByText('Claude $11.00 · Codex $2.00')).toBeTruthy();
    expect(within(tile('Sessions')).getByText('Across 2 projects')).toBeTruthy();
    // 24 cache reads of 24,264 input tokens.
    expect(within(tile('Tokens')).getByText('Cache hits 98.9%')).toBeTruthy();
  });

  it('draws both plan cards with the same rows, shortest window first', async () => {
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Max 5x');

    const cards = Array.from(document.querySelectorAll('.ai-quota-card'));
    expect(cards).toHaveLength(2);
    const rows = cards.map((card) => card.querySelectorAll('.ai-quota-rows > li'));
    expect(rows.map((r) => r.length)).toEqual([3, 3]);
    expect(within(rows[0][0] as HTMLElement).getByText('5-hour window')).toBeTruthy();
    expect(within(rows[0][0] as HTMLElement).getByText('45%')).toBeTruthy();
    expect(within(rows[1][0] as HTMLElement).getByText('100%')).toBeTruthy();
    expect(rows[1][1].className).toContain('ai-quota-row-empty');
  });

  it('asks for the chosen period and app and says where the period starts', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AiUsageSection isMobile={false} />);
    await screen.findByText('Since 2026-10-01');

    await user.click(screen.getByText('This week'));
    await user.click(screen.getByText('Codex', { selector: '.ant-segmented-item-label' }));

    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith(
        '/panel/api/aiUsage/overview',
        { period: 'week', deviceId: 0, app: 'codex' },
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
