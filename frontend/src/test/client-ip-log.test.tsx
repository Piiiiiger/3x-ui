import { act, fireEvent, renderHook, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ClientIpLogModal from '@/components/clients/ClientIpLog';
import { useClientIpLog } from '@/hooks/useClientIpLog';
import type { ClientIpInfo } from '@/lib/clients/ip-log';
import IpLimitExemptHosts from '@/pages/settings/IpLimitExemptHosts';
import { HttpUtil } from '@/utils';
import { renderWithProviders } from './test-utils';

const NOW_MS = 1_800_000_000_000;

function entry(ip: string, extra: Partial<ClientIpInfo> = {}): ClientIpInfo {
  return { ip, time: '', node: '', exempt: '', exemptHost: '', bannedUntil: 0, ...extra };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ClientIpLogModal', () => {
  it('tells exempt and banned addresses apart and offers to lift a ban', () => {
    const onUnban = vi.fn();
    renderWithProviders(
      <ClientIpLogModal
        open
        email="sam"
        ips={[
          entry('203.0.113.7', { exempt: 'host', exemptHost: 'hk-relay' }),
          entry('192.0.2.200', { exempt: 'host' }),
          entry('198.51.100.1', { exempt: 'allowlist' }),
          entry('10.10.16.1', { exempt: 'private' }),
          entry('198.51.100.9', { bannedUntil: NOW_MS / 1000 + 14 * 60 + 1 }),
        ]}
        bans={[{ network: '198.51.100.9', bannedAt: 0, expiresAt: NOW_MS / 1000 + 14 * 60 + 1 }]}
        nowMs={NOW_MS}
        loading={false}
        clearing={false}
        unbanning={null}
        onRefresh={() => {}}
        onClear={() => {}}
        onUnban={onUnban}
        onClose={() => {}}
      />,
    );

    for (const label of ['Server hk-relay', 'This panel', 'Allowlisted', 'Private address']) {
      expect(screen.getByText(label)).toBeTruthy();
    }
    expect(screen.getAllByText('Banned · 15 min left')).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Unban' }));
    expect(onUnban).toHaveBeenCalledWith('198.51.100.9');
  });
});

describe('useClientIpLog', () => {
  it('drops a lifted ban from the list', async () => {
    let banned = true;
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url: string, data?: unknown) => {
      if (url === '/panel/api/clients/ips/sam') return { success: true, msg: '', obj: [] };
      if (url === '/panel/api/clients/ipBans/sam') {
        return {
          success: true,
          msg: '',
          obj: banned ? [{ network: '198.51.100.9', bannedAt: 1, expiresAt: 2 }] : [],
        };
      }
      const body = data as { network?: string } | undefined;
      if (url === '/panel/api/clients/unbanIp/sam' && body?.network === '198.51.100.9') {
        banned = false;
        return { success: true, msg: '', obj: null };
      }
      return { success: false, msg: 'unexpected request', obj: null };
    });
    const { result } = renderHook(() => useClientIpLog('sam'));

    await act(() => result.current.load());
    expect(result.current.bans.map((b) => b.network)).toEqual(['198.51.100.9']);
    await act(() => result.current.unban('198.51.100.9'));
    expect(result.current.bans).toEqual([]);
  });
});

describe('IpLimitExemptHosts', () => {
  it('lists every exempt server, this panel included', async () => {
    vi.spyOn(HttpUtil, 'post').mockResolvedValue({
      success: true,
      msg: '',
      obj: [
        { name: '', addresses: ['192.0.2.200'] },
        { name: 'hk-relay', addresses: ['203.0.113.7', '2001:db8:1:2::/64'] },
      ],
    });
    renderWithProviders(<IpLimitExemptHosts />);

    await waitFor(() => expect(screen.getByText('hk-relay')).toBeTruthy());
    for (const text of ['This panel', '192.0.2.200', '203.0.113.7', '2001:db8:1:2::/64']) {
      expect(screen.getByText(text)).toBeTruthy();
    }
  });
});
