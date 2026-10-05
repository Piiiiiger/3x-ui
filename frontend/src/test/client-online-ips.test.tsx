import { act, fireEvent, renderHook, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ClientOnlineIpsModal from '@/components/clients/ClientOnlineIps';
import type { ClientOnlineIps } from '@/generated/zod';
import { useClientOnlineIps } from '@/hooks/useClientOnlineIps';
import { IpSlots } from '@/pages/clients/ClientCells';
import PortalOnlineIps from '@/pages/sub/portal/PortalOnlineIps';
import { HttpUtil } from '@/utils';
import { renderWithProviders } from './test-utils';

const NOW_MS = 1_800_000_000_000;
const IN_15_MIN = NOW_MS / 1000 + 14 * 60 + 1;

const sample: ClientOnlineIps = {
  limit: 3,
  count: 1,
  online: [
    {
      network: '198.51.100.20',
      addresses: ['198.51.100.20'],
      servers: ['HK relay'],
      lastSeen: NOW_MS / 1000 - 5,
      counted: true,
    },
    {
      network: '198.51.100.200',
      addresses: ['198.51.100.200'],
      servers: ['SG exit'],
      lastSeen: NOW_MS / 1000 - 9,
      counted: false,
    },
  ],
  bans: [
    {
      id: 1,
      email: 'sam',
      network: '198.51.100.9',
      bannedAt: NOW_MS / 1000 - 60,
      expiresAt: IN_15_MIN,
    },
  ],
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ClientOnlineIpsModal', () => {
  it('shows the slots in use, each IP with its server, and bans with time left and Unban', () => {
    const onUnban = vi.fn();
    renderWithProviders(
      <ClientOnlineIpsModal
        open
        email="sam"
        data={sample}
        nowMs={NOW_MS}
        loading={false}
        unbanning={null}
        onRefresh={() => {}}
        onUnban={onUnban}
        onClose={() => {}}
      />,
    );

    expect(screen.getByText('1/3')).toBeTruthy();
    expect(screen.getByText('198.51.100.20')).toBeTruthy();
    expect(screen.getByText('HK relay')).toBeTruthy();
    expect(screen.getByText('Allowlisted · not counted')).toBeTruthy();
    expect(screen.getByText('Banned · 15 min left')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Unban' }));
    expect(onUnban).toHaveBeenCalledWith('198.51.100.9');
  });
});

describe('IpSlots', () => {
  it('shows slots in use against the limit and turns red while a ban runs', () => {
    const { container, rerender } = renderWithProviders(<IpSlots count={2} limit={3} bans={0} />);
    expect(screen.getByText('2/3')).toBeTruthy();
    expect(container.querySelector('.ant-tag-red')).toBeNull();
    rerender(<IpSlots count={2} limit={3} bans={1} />);
    expect(container.querySelector('.ant-tag-red')?.textContent).toBe('2/3');
  });

  it('stays out of the way for a client without a limit and nothing online', () => {
    const { container } = renderWithProviders(<IpSlots count={0} limit={0} bans={0} />);
    expect(container.textContent).toBe('');
  });
});

describe('useClientOnlineIps', () => {
  it('drops a lifted ban from the view', async () => {
    let banned = true;
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url: string, data?: unknown) => {
      if (url === '/panel/api/clients/onlineIps/sam') {
        return { success: true, msg: '', obj: { ...sample, bans: banned ? sample.bans : [] } };
      }
      const body = data as { network?: string } | undefined;
      if (url === '/panel/api/clients/unbanIp/sam' && body?.network === '198.51.100.9') {
        banned = false;
        return { success: true, msg: '', obj: null };
      }
      return { success: false, msg: 'unexpected request', obj: null };
    });
    const { result } = renderHook(() => useClientOnlineIps('sam'));

    await act(() => result.current.load());
    expect(result.current.data?.bans.map((b) => b.network)).toEqual(['198.51.100.9']);
    await act(() => result.current.unban('198.51.100.9'));
    expect(result.current.data?.bans).toEqual([]);
  });
});

describe('PortalOnlineIps', () => {
  it('shows the person their slots, IPs and bans with the reason, with nothing to act on', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(NOW_MS);
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      if (String(input) === '/x/portal/online-ips') {
        return new Response(JSON.stringify(sample), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        });
      }
      return new Response('', { status: 404 });
    });
    renderWithProviders(<PortalOnlineIps base="/x/portal" email="sam" onSessionEnded={() => {}} />);

    await waitFor(() => expect(screen.getByText('1/3')).toBeTruthy());
    expect(screen.getByText('198.51.100.20')).toBeTruthy();
    expect(screen.getByText('Banned · 15 min left')).toBeTruthy();
    expect(
      screen.getByText(
        'More than 3 IPs were online at once, so this IP is blocked until the time runs out.',
      ),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Unban' })).toBeNull();
  });

  it('hands a dead session back to the sign-in form', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 401 }));
    const ended = vi.fn();
    renderWithProviders(<PortalOnlineIps base="/x/portal" email="sam" onSessionEnded={ended} />);
    await waitFor(() => expect(ended).toHaveBeenCalled());
  });
});
