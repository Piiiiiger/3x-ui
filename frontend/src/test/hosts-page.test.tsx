import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { keys } from '@/api/queryKeys';
import type { ProbeLinkView, ProbeOverview, ProbeServer } from '@/generated/zod';
import NodesPage from '@/pages/nodes/NodesPage';
import { HttpUtil, Msg } from '@/utils';
import { chooseSelectOption, makeTestQueryClient, renderWithProviders } from './test-utils';

// The top bar has its own tests and needs settings the page itself does not.
vi.mock('@/layouts/AppNav', () => ({ default: () => null }));

// antd names a button by its icon first ("setting Probe settings"), hence the
// patterns anchored at the end wherever a toolbar button is looked up.

const GIB = 1024 ** 3;
const NOON_UTC = Date.UTC(2026, 9, 2, 12, 0, 0);

// What Lite lists for a server whose agent has never reported.
function server(overrides: Partial<ProbeServer>): ProbeServer {
  return {
    id: 'uuid',
    name: 'server',
    region: '',
    os: '',
    arch: '',
    virtualization: '',
    cpuCores: 0,
    status: 'unknown',
    updatedAt: 0,
    cpu: 0,
    memUsed: 0,
    memTotal: 0,
    diskUsed: 0,
    diskTotal: 0,
    load1: 0,
    load5: 0,
    load15: 0,
    netIn: 0,
    netOut: 0,
    netTotalUp: 0,
    netTotalDown: 0,
    uptime: 0,
    trafficLimit: 0,
    trafficUsed: 0,
    pings: [],
    linked: false,
    nodeId: 0,
    nodeName: '',
    ...overrides,
  };
}

// Linked to the panel's own host.
const losAngeles = server({
  id: 'uuid-la',
  name: '洛杉矶-Alpha',
  region: '🇺🇸',
  status: 'online',
  updatedAt: NOON_UTC,
  cpu: 3.2,
  memUsed: 1 * GIB,
  memTotal: 2 * GIB,
  diskUsed: 5 * GIB,
  diskTotal: 20 * GIB,
  netIn: 4096,
  netOut: 2048,
  uptime: 86400,
  trafficLimit: 100 * GIB,
  trafficUsed: 25 * GIB,
  linked: true,
  nodeId: 0,
});

const hongKong = server({
  id: 'uuid-hk',
  name: '香港-Bravo',
  region: '🇭🇰',
  os: 'Ubuntu 24.04.3 LTS',
  arch: 'arm64',
  virtualization: 'none',
  cpuCores: 2,
  status: 'online',
  updatedAt: NOON_UTC,
  memTotal: 1 * GIB,
  diskTotal: 30 * GIB,
  netIn: 1024,
  netOut: 1024,
  trafficUsed: 3 * GIB,
  linked: true,
  nodeId: 2,
  nodeName: 'edge-hk',
});

// Not linked, so node id 0 here does not mean the panel's own host; and a
// server that is not online arrives without a usage figure.
const london = server({
  id: 'uuid-uk',
  name: '英国-Charlie',
  region: '🇬🇧',
  status: 'offline',
  updatedAt: NOON_UTC,
  trafficLimit: 850 * GIB,
});

function overview(overrides: Partial<ProbeOverview> = {}): ProbeOverview {
  return {
    configured: true,
    publicUrl: '',
    fetchedAt: NOON_UTC,
    stale: false,
    error: '',
    servers: [losAngeles, hongKong, london],
    ...overrides,
  };
}

const notConfigured = overview({ configured: false, fetchedAt: 0, servers: [] });

const links: ProbeLinkView[] = [
  { nodeId: 0, nodeName: '', address: '', serverId: 'uuid-la', serverName: '洛杉矶-Alpha' },
  { nodeId: 2, nodeName: 'edge-hk', address: '203.0.113.7', serverId: '', serverName: '' },
];

const NODES = [
  {
    id: 2,
    name: 'edge-hk',
    kind: 'agent',
    enable: true,
    status: 'online',
    address: '203.0.113.7',
    xrayVersion: '25.10.1',
    cpuPct: 5,
    memPct: 30,
    uptimeSecs: 3600,
    lastHeartbeat: 0,
  },
  {
    id: 3,
    name: 'edge-sg',
    kind: 'agent',
    enable: true,
    status: 'online',
    address: '198.51.100.9',
    xrayVersion: '25.10.1',
    cpuPct: 7.5,
    memPct: 41,
    uptimeSecs: 7200,
    lastHeartbeat: 0,
  },
];

const INBOUND_OPTIONS = [
  { id: 11, nodeId: 2, enable: true, remark: 'hk-reality' },
  { id: 12, nodeId: 2, enable: false, remark: 'hk-ws' },
  { id: 13, enable: true, remark: 'local-reality' },
];

const STATUS = {
  cpu: 12,
  mem: { current: 1 * GIB, total: 4 * GIB },
  disk: { current: 10 * GIB, total: 40 * GIB },
  netIO: { up: 1000, down: 2000 },
  uptime: 7200,
  publicIP: { ipv4: '192.0.2.1', ipv6: 'N/A' },
  xray: { state: 'running', errorMsg: '', version: '25.10.2', color: 'green' },
};

function serve(answer: () => Msg<unknown>) {
  return vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) => {
    if (url === '/panel/api/probe/servers') return answer();
    if (url === '/panel/api/probe/settings') return new Msg(true, '', { url: '', publicUrl: '' });
    if (url === '/panel/api/probe/links') return new Msg(true, '', links);
    if (url === '/panel/api/nodes/list') return new Msg(true, '', NODES);
    if (url === '/panel/api/inbounds/options') return new Msg(true, '', INBOUND_OPTIONS);
    if (url === '/panel/api/server/status') return new Msg(true, '', STATUS);
    if (url === '/panel/api/server/getPanelUpdateInfo') return new Msg(true, '', {});
    return new Msg(false, `unexpected GET ${url}`);
  });
}

function renderPage(options?: { queryClient?: QueryClient }) {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/nodes']}>
      <NodesPage />
    </MemoryRouter>,
    options,
  );
}

function hostCards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('.host-card'));
}

function monitorCards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('.probe-card'));
}

function hostCard(name: string): HTMLElement {
  return screen
    .getByText(name, { selector: '.host-card-name' })
    .closest('.host-card') as HTMLElement;
}

// jsdom never ends an animation, so a modal that was closed stays in its leave motion.
function isClosing(dialog: HTMLElement): boolean {
  return dialog.classList.contains('ant-zoom-leave');
}

function pollsOf(get: ReturnType<typeof serve>): number {
  return get.mock.calls.filter(([url]) => url === '/panel/api/probe/servers').length;
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  localStorage.clear();
});

describe('the hosts page (服务管理)', () => {
  it('asks for the Lite address while the probe is not set up, and opens its settings from there', async () => {
    serve(() => new Msg(true, '', notConfigured));
    renderPage();

    const notice = (await screen.findByText('The probe is not set up yet')).closest(
      '.ant-alert',
    ) as HTMLElement;
    await waitFor(() => expect(hostCards()).toHaveLength(3));
    expect(monitorCards()).toHaveLength(0);
    expect(within(hostCard('edge-hk')).getByText('Not linked to a probe server')).toBeTruthy();
    expect(screen.queryByText('Open public page')).toBeNull();
    // Without a Lite address there is no server to link a host to.
    expect(screen.getByRole('button', { name: /Link hosts$/ }).hasAttribute('disabled')).toBe(true);

    fireEvent.click(within(notice).getByRole('button', { name: /Probe settings$/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Probe settings' });
    expect(isClosing(dialog)).toBe(false);
  });

  // Lite being down is an answer of the API, not a failed request.
  it('says why Lite could not be read when there is nothing to show', async () => {
    serve(
      () =>
        new Msg(true, '', overview({ error: 'Lite is not reachable', fetchedAt: 0, servers: [] })),
    );
    renderPage();

    const alert = (await screen.findByText('The Lite monitor could not be read')).closest(
      '.ant-alert',
    ) as HTMLElement;
    expect(within(alert).getByText('Lite is not reachable')).toBeTruthy();
    expect(alert.className).toContain('ant-alert-error');
    expect(monitorCards()).toHaveLength(0);
  });

  // The modal would call every stored link lost, and clearing them would lose them.
  it('tells Link hosts that the servers of Lite are not known while it cannot be read', async () => {
    serve(
      () =>
        new Msg(true, '', overview({ error: 'Lite is not reachable', fetchedAt: 0, servers: [] })),
    );
    renderPage();
    await screen.findByText('The Lite monitor could not be read');

    fireEvent.click(screen.getByRole('button', { name: /Link hosts$/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Link hosts' });
    await within(dialog).findByText('203.0.113.7');

    expect(
      within(dialog).getByText('Lite cannot be read right now, so its servers cannot be listed.'),
    ).toBeTruthy();
    expect(within(dialog).queryByText('Lite no longer lists this server')).toBeNull();
  });

  it('keeps the last servers on screen with the time of the data while Lite fails', async () => {
    serve(() => new Msg(true, '', overview({ stale: true, error: 'Lite did not answer in 3s' })));
    renderPage();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/Showing data from \d{2}:\d{2}:\d{2}/);
    expect(alert.textContent).toContain('Lite did not answer in 3s');
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
  });

  it("puts each linked server's figures on its host, and the rest under 仅监控", async () => {
    serve(() => new Msg(true, '', overview({ publicUrl: 'https://probe.example.com' })));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
    expect(screen.queryByRole('alert')).toBeNull();

    const local = hostCard('Local panel');
    expect(within(local).getByRole('progressbar', { name: 'Traffic quota 25.0 %' })).toBeTruthy();
    expect(within(local).getByText('25.00 GB / 100.00 GB')).toBeTruthy();
    expect(within(local).getByText('Xray 25.10.2')).toBeTruthy();

    // Without a limit the quota row stays, with the usage and an empty bar.
    const hk = hostCard('edge-hk');
    expect(
      within(hk)
        .getByRole('progressbar', { name: 'Traffic quota 3.00 GB / Unlimited' })
        .getAttribute('aria-valuenow'),
    ).toBe('0');

    // No probe server: the heartbeat still gives the CPU.
    const sg = hostCard('edge-sg');
    expect(within(sg).getByRole('progressbar', { name: 'CPU 7.5 %' })).toBeTruthy();
    expect(within(sg).getByText('Not linked to a probe server')).toBeTruthy();

    const [uk] = monitorCards();
    expect(within(uk).getByText('英国-Charlie')).toBeTruthy();
    // The usage of a server that is not online is not known: no bar at 0 of 850 GB.
    expect(within(uk).queryAllByRole('progressbar')).toHaveLength(0);

    const link = screen.getByRole('link', { name: /Open public page$/ });
    expect(link.getAttribute('href')).toBe('https://probe.example.com');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  // 妙妙屋X's cards line up because each has every section, filled or not.
  it('gives every host card the same sections, with figures or without', async () => {
    serve(() => new Msg(true, '', overview()));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));
    await waitFor(() =>
      expect(within(hostCard('edge-hk')).queryByText('Not linked to a probe server')).toBeNull(),
    );

    for (const card of hostCards()) {
      expect(within(card).getAllByRole('progressbar')).toHaveLength(4);
      expect(card.querySelectorAll('.host-card-network')).toHaveLength(1);
      expect(card.querySelectorAll('.host-card-foot')).toHaveLength(1);
    }
    expect(within(hostCard('edge-hk')).getByText('1/2 nodes')).toBeTruthy();
    expect(within(hostCard('Local panel')).getByText('1/1 nodes')).toBeTruthy();
  });

  it.each([
    ['edge-hk', '/panel/api/nodes/restartXray/2'],
    ['Local panel', '/panel/api/server/restartXrayService'],
  ])('restarts the Xray of %s after asking', async (name, url) => {
    serve(() => new Msg(true, '', overview()));
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, ''));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));

    fireEvent.click(within(hostCard(name)).getByRole('button', { name: 'Restart Xray' }));
    const dialog = await screen.findByRole('dialog', { name: `Restart Xray on ${name}?` });
    expect(post).not.toHaveBeenCalledWith(url);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart Xray' }));
    await waitFor(() => expect(post).toHaveBeenCalledWith(url));
  });

  // Disabling stops the panel managing a host, so it asks first; the local panel has no switch.
  it('disables a host only once the admin confirms', async () => {
    serve(() => new Msg(true, '', overview()));
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, ''));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));
    expect(within(hostCard('Local panel')).queryByRole('button', { name: 'Disable' })).toBeNull();

    fireEvent.click(within(hostCard('edge-hk')).getByRole('button', { name: 'Disable' }));
    const confirm = await screen.findByRole('tooltip');
    expect(post).not.toHaveBeenCalledWith('/panel/api/nodes/setEnable/2', { enable: false });
    fireEvent.click(within(confirm).getByRole('button', { name: 'Disable' }));
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/panel/api/nodes/setEnable/2', { enable: false }),
    );
  });

  it('hides the addresses until asked, and remembers the choice', async () => {
    serve(() => new Msg(true, '', overview()));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));
    const address = within(hostCard('edge-hk')).getByText('203.0.113.7');
    expect(address.className).toBe('address-hidden');

    fireEvent.click(screen.getByRole('button', { name: /Show IP$/ }));
    expect(within(hostCard('edge-hk')).getByText('203.0.113.7').className).toBe('address-visible');
    expect(localStorage.getItem('hosts-show-address')).toBe('true');
  });

  it('switches to the list of hosts', async () => {
    serve(() => new Msg(true, '', overview()));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));

    fireEvent.click(screen.getByTitle('List'));
    const table = await screen.findByRole('table');
    expect(within(table).getByRole('link', { name: 'edge-hk' })).toBeTruthy();
    expect(hostCards()).toHaveLength(0);
    // The monitored servers stay below either view.
    expect(monitorCards()).toHaveLength(1);
  });

  it('lists no monitored servers when Lite has none, without an error', async () => {
    serve(() => new Msg(true, '', overview({ servers: [] })));
    renderPage();
    await waitFor(() => expect(hostCards()).toHaveLength(3));

    expect(screen.queryByText('Monitored only')).toBeNull();
    expect(screen.queryByText('The Lite monitor could not be read')).toBeNull();
  });

  it('reports a failed request with its message, and asks again on Refresh', async () => {
    let failing = true;
    serve(() =>
      failing
        ? new Msg(false, 'Something went wrong (database is locked)')
        : new Msg(true, '', overview()),
    );
    renderPage();

    const alert = (await screen.findByText('Something went wrong (database is locked)')).closest(
      '.ant-alert',
    ) as HTMLElement;
    expect(monitorCards()).toHaveLength(0);

    failing = false;
    fireEvent.click(within(alert).getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
  });

  // A tab left open across an upgrade may be sent a status its code has never heard of.
  it('reports an answer it does not understand instead of drawing it', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const rebooting = { ...london, status: 'rebooting' };
    serve(() => new Msg(true, '', { ...overview(), servers: [rebooting] }));
    renderPage();

    expect(await screen.findByText('probe/servers response failed validation')).toBeTruthy();
    expect(monitorCards()).toHaveLength(0);
  });

  // The page polls every 3 s: one dropped request must not blank a screen of servers.
  it('keeps the servers when a later poll fails, marked with the time they are from', async () => {
    let failing = false;
    serve(() => (failing ? new Msg(false, 'Request failed') : new Msg(true, '', overview())));
    const queryClient = makeTestQueryClient();
    renderPage({ queryClient });
    await waitFor(() => expect(monitorCards()).toHaveLength(1));

    failing = true;
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: keys.probe.servers() });
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/Showing data from \d{2}:\d{2}:\d{2}/);
    expect(alert.textContent).toContain('Request failed');
    expect(monitorCards()).toHaveLength(1);
  });

  // The figures are live: a page that asked once would show the load of minutes ago.
  it('asks again every three seconds, but not while the tab is hidden', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const get = serve(() => new Msg(true, '', overview()));
    renderPage();
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
    expect(pollsOf(get)).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(pollsOf(get)).toBe(2);

    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(9000);
    });
    expect(pollsOf(get)).toBe(2);
  });

  // The panel keeps an answer fresh for 30 s by default; these figures are stale at once.
  it('asks at once when the page is opened again', async () => {
    const get = serve(() => new Msg(true, '', overview()));
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    const first = renderPage({ queryClient });
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
    first.unmount();

    renderPage({ queryClient });
    await waitFor(() => expect(pollsOf(get)).toBe(2));
  });

  it.each([
    ['Link hosts', /Link hosts$/],
    ['Probe settings', /Probe settings$/],
  ])('closes the "%s" modal when the admin cancels it', async (title, button) => {
    serve(() => new Msg(true, '', overview()));
    renderPage();
    await waitFor(() => expect(monitorCards()).toHaveLength(1));

    fireEvent.click(screen.getByRole('button', { name: button }));
    const dialog = await screen.findByRole('dialog', { name: title });
    fireEvent.click(await within(dialog).findByRole('button', { name: 'Cancel' }));
    expect(isClosing(dialog)).toBe(true);
  });

  // Without the refresh the page stays on "not set up" until the next poll.
  it('shows the servers as soon as the Lite address is saved', async () => {
    let configured = false;
    serve(() => new Msg(true, '', configured ? overview() : notConfigured));
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url: string) => {
      if (url !== '/panel/api/probe/settings') return new Msg(true, '', []);
      configured = true;
      return new Msg(true, '', { url: 'http://127.0.0.1:27777', publicUrl: '' });
    });
    renderPage();

    await screen.findByText('The probe is not set up yet');
    fireEvent.click(screen.getAllByRole('button', { name: /Probe settings$/ })[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Probe settings' });
    fireEvent.change(await within(dialog).findByLabelText('Lite address'), {
      target: { value: 'http://127.0.0.1:27777' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(monitorCards()).toHaveLength(1));
    expect(await screen.findByText('Probe settings saved')).toBeTruthy();
    expect(isClosing(dialog)).toBe(true);
  });

  // Linking is what puts a server's figures on its host, and in its clients' portal.
  it('moves a server onto its host as soon as the links are saved', async () => {
    let linked = false;
    serve(
      () =>
        new Msg(
          true,
          '',
          overview({
            servers: [
              losAngeles,
              linked ? hongKong : { ...hongKong, linked: false, nodeId: 0, nodeName: '' },
            ],
          }),
        ),
    );
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url: string) => {
      if (url !== '/panel/api/probe/links') return new Msg(true, '', []);
      linked = true;
      return new Msg(true, '', links);
    });
    renderPage();
    await waitFor(() => expect(monitorCards()).toHaveLength(1));
    expect(within(monitorCards()[0]).getByText('香港-Bravo')).toBeTruthy();
    expect(within(hostCard('edge-hk')).getByText('Not linked to a probe server')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: /Link hosts$/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Link hosts' });
    await within(dialog).findByText('203.0.113.7');
    chooseSelectOption('probe-link-2', '🇭🇰 香港-Bravo');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(monitorCards()).toHaveLength(0));
    expect(
      within(hostCard('edge-hk')).getByRole('progressbar', {
        name: 'Traffic quota 3.00 GB / Unlimited',
      }),
    ).toBeTruthy();
    expect(await screen.findByText('Links saved')).toBeTruthy();
    expect(isClosing(dialog)).toBe(true);
  });
});
