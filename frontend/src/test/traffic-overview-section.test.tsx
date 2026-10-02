import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import TrafficOverviewSection from '@/pages/index/TrafficOverviewSection';
import type { TrafficOverview } from '@/generated/zod';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from '@/test/test-utils';

// jsdom has no canvas for the chart, and these tests are about the cards around it.
vi.mock('@/pages/index/DailyTrafficCard', () => ({ default: () => null }));

const GiB = 1024 ** 3;
const TiB = 1024 * GiB;

function overview(over: Partial<TrafficOverview> = {}): TrafficOverview {
  return {
    servers: {
      configured: true,
      error: '',
      quotaBytes: 2 * TiB,
      usedBytes: 512 * GiB,
      remainingBytes: 1536 * GiB,
      unlimited: 1,
      unlinked: 1,
    },
    hosts: [
      { nodeId: 0, name: '', linked: true, quotaBytes: 2 * TiB, usedBytes: 511 * GiB },
      { nodeId: 2, name: 'edge-2', linked: true, quotaBytes: 0, usedBytes: 1 * GiB },
      { nodeId: 3, name: 'edge-3', linked: false, quotaBytes: 0, usedBytes: 0 },
    ],
    daily: [{ day: '2026-10-01', up: 1, down: 2 }],
    period: 'month',
    periodStart: '2026-10-01',
    hostRanking: [
      { nodeId: 2, name: 'edge-2', up: 3 * GiB, down: 4 * GiB },
      { nodeId: 0, name: '', up: 1 * GiB, down: 0 },
      { nodeId: 3, name: 'edge-3', up: 0, down: 0 },
    ],
    userRanking: ['amy', 'bob', 'cat', 'dan', 'eve', 'fay', 'gus'].map((email, i) => ({
      email,
      up: (7 - i) * GiB,
      down: 0,
    })),
    users: 7,
    ...over,
  };
}

let current: TrafficOverview;
let getSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  current = overview();
  getSpy = vi.spyOn(HttpUtil, 'get').mockImplementation(async (url, params) => {
    if (url === '/panel/api/traffic/overview') {
      const period = (params as { period: TrafficOverview['period'] }).period;
      return new Msg(true, '', { ...current, period });
    }
    if (url === '/panel/api/nodes/list') {
      return new Msg(true, '', [
        { id: 2, name: 'edge-2', status: 'online', netUp: 1024, netDown: 1024 },
        { id: 3, name: 'edge-3', status: 'offline', netUp: 9999, netDown: 9999 },
      ]);
    }
    if (url === '/panel/api/server/status') {
      return new Msg(true, '', { netIO: { up: 1024, down: 3 * 1024 } });
    }
    return new Msg(false, 'unexpected ' + url);
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function tile(label: string) {
  return screen.getByText(label).closest('.ov-tile') as HTMLElement;
}

describe('TrafficOverviewSection', () => {
  it("fills the cards with the hosts' quotas and their summed live speed", async () => {
    renderWithProviders(<TrafficOverviewSection isMobile={false} />);

    await waitFor(() => expect(within(tile('Total quota')).getByText('2.00')).toBeTruthy());
    expect(within(tile('Total quota')).getByText('TB')).toBeTruthy();
    expect(
      within(tile('Total quota')).getByText(
        '1 with a quota · 1 unlimited · 1 not linked to the probe',
      ),
    ).toBeTruthy();
    expect(within(tile('Remaining')).getByText('1.50')).toBeTruthy();
    // The panel's own 1 KB/s up and 3 KB/s down, plus the online host's 1 KB/s each way.
    await waitFor(() => expect(within(tile('Live speed')).getByText(/2\.00 KB\/s/)).toBeTruthy());
    expect(within(tile('Live speed')).getByText(/4\.00 KB\/s/)).toBeTruthy();
  });

  it('asks for the chosen period and says where it starts', async () => {
    const user = userEvent.setup();
    renderWithProviders(<TrafficOverviewSection isMobile={false} />);
    await screen.findByText('Since the 1st, 00:00');

    await user.click(screen.getByText('This week'));

    await screen.findByText('Since Monday 00:00');
    expect(getSpy).toHaveBeenCalledWith(
      '/panel/api/traffic/overview',
      { period: 'week' },
      { silent: true },
    );
  });

  it('shows the five busiest users and opens the whole list', async () => {
    const user = userEvent.setup();
    renderWithProviders(<TrafficOverviewSection isMobile={false} />);
    const card = (await screen.findByText('By user')).closest('.ov-rank') as HTMLElement;

    expect(within(card).getByText('eve')).toBeTruthy();
    expect(within(card).queryByText('fay')).toBeNull();
    expect(
      within(card).getByText('7 users in all; the full list is at the top right'),
    ).toBeTruthy();

    await user.click(within(card).getByRole('button', { name: 'By user: Show all' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('gus')).toBeTruthy();
    expect(within(dialog).getAllByRole('row')).toHaveLength(8);
  });

  it('names the panel itself and marks a host without a probe link', async () => {
    renderWithProviders(<TrafficOverviewSection isMobile={false} />);
    const table = (await screen.findByText('Hosts')).closest('.ov-hosts') as HTMLElement;

    const rows = within(table).getAllByRole('row').slice(1);
    expect(within(rows[0]).getByText('Local panel')).toBeTruthy();
    expect(within(rows[1]).getByText('Unlimited')).toBeTruthy();
    expect(within(rows[2]).getByText('Not linked')).toBeTruthy();
  });

  it('shows no quota numbers, and why, without a probe', async () => {
    current = overview({
      servers: {
        configured: false,
        error: '',
        quotaBytes: 0,
        usedBytes: 0,
        remainingBytes: 0,
        unlimited: 0,
        unlinked: 3,
      },
    });
    renderWithProviders(<TrafficOverviewSection isMobile={false} />);

    await screen.findByText('Set up the probe to see quotas');
    expect(within(tile('Total quota')).getByText('—')).toBeTruthy();
  });
});
