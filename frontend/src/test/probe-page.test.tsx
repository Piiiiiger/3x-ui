import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { keys } from '@/api/queryKeys';
import type { ProbeLinkView, ProbeOverview, ProbeServer } from '@/generated/zod';
import ProbePage from '@/pages/probe/ProbePage';
import { HttpUtil, Msg } from '@/utils';
import { chooseSelectOption, makeTestQueryClient, renderWithProviders } from './test-utils';

// The top bar has its own tests and needs a router the page itself does not.
vi.mock('@/layouts/AppNav', () => ({ default: () => null }));

// antd names a button by its icon first ("setting Settings"), hence the patterns
// anchored at the end wherever a button of the page header is looked up.

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

const losAngeles = server({
  id: 'uuid-la',
  name: '洛杉矶-Alpha',
  region: '🇺🇸',
  os: 'Debian GNU/Linux 13 (trixie)',
  arch: 'amd64',
  virtualization: 'kvm',
  cpuCores: 1,
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

function serve(answer: () => Msg<unknown>) {
  return vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) => {
    if (url === '/panel/api/probe/servers') return answer();
    if (url === '/panel/api/probe/settings') return new Msg(true, '', { url: '', publicUrl: '' });
    if (url === '/panel/api/probe/links') return new Msg(true, '', links);
    return new Msg(false, `unexpected GET ${url}`);
  });
}

function cards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>('.probe-card'));
}

function resultOf(title: string): HTMLElement {
  return screen.getByText(title).closest('.ant-result') as HTMLElement;
}

function statistic(title: string): string | null | undefined {
  return screen.getByText(title).closest('.ant-statistic')?.querySelector('.ant-statistic-content')
    ?.textContent;
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
});

describe('ProbePage', () => {
  it('asks for the Lite address while the probe is not set up, and opens Settings from there', async () => {
    serve(() => new Msg(true, '', notConfigured));
    renderWithProviders(<ProbePage />);

    await screen.findByText('The probe is not set up yet');
    expect(cards()).toHaveLength(0);
    expect(screen.queryByText('Open public page')).toBeNull();
    // Without a Lite address there is no server to link a host to.
    expect(screen.getByRole('button', { name: /Link hosts$/ }).hasAttribute('disabled')).toBe(true);

    fireEvent.click(
      within(resultOf('The probe is not set up yet')).getByRole('button', { name: /Settings$/ }),
    );
    const dialog = await screen.findByRole('dialog', { name: 'Probe settings' });
    expect(isClosing(dialog)).toBe(false);
  });

  // Lite being down is an answer of the API, not a failed request.
  it('says why Lite could not be read when there is nothing to show', async () => {
    serve(
      () =>
        new Msg(true, '', overview({ error: 'Lite is not reachable', fetchedAt: 0, servers: [] })),
    );
    renderWithProviders(<ProbePage />);

    await screen.findByText('The Lite monitor could not be read');
    const failure = resultOf('The Lite monitor could not be read');
    expect(within(failure).getByText('Lite is not reachable')).toBeTruthy();
    expect(failure.className).toContain('ant-result-error');
    expect(cards()).toHaveLength(0);
  });

  // The modal would call every stored link lost, and clearing them would lose them.
  it('tells Link hosts that the servers of Lite are not known while it cannot be read', async () => {
    serve(
      () =>
        new Msg(true, '', overview({ error: 'Lite is not reachable', fetchedAt: 0, servers: [] })),
    );
    renderWithProviders(<ProbePage />);
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
    renderWithProviders(<ProbePage />);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/Showing data from \d{2}:\d{2}:\d{2}/);
    expect(alert.textContent).toContain('Lite did not answer in 3s');
    expect(cards()).toHaveLength(3);
  });

  it('shows every server with its system line, its linked host and its quota', async () => {
    serve(() => new Msg(true, '', overview({ publicUrl: 'https://probe.example.com' })));
    renderWithProviders(<ProbePage />);

    await waitFor(() => expect(cards()).toHaveLength(3));
    expect(screen.queryByRole('alert')).toBeNull();
    const [la, hk, uk] = cards();

    expect(within(la).getByText('洛杉矶-Alpha')).toBeTruthy();
    expect(
      within(la).getByText('Debian GNU/Linux 13 (trixie) · amd64 · kvm · 1-core'),
    ).toBeTruthy();
    expect(within(la).getByText('Local panel')).toBeTruthy();
    expect(within(la).getByRole('progressbar', { name: 'Traffic quota 25.0 %' })).toBeTruthy();
    expect(within(la).getByText('25.00 GB / 100.00 GB')).toBeTruthy();

    // "none" is what a bare-metal server reports; it is not a kind of virtualization.
    expect(within(hk).getByText('Ubuntu 24.04.3 LTS · arm64 · 2-core')).toBeTruthy();
    expect(within(hk).getByText('edge-hk')).toBeTruthy();
    // Without a limit the row stays, so every card ends alike: the usage, and an empty bar.
    expect(
      within(hk)
        .getByRole('progressbar', { name: 'Traffic quota 3.00 GB / Unlimited' })
        .getAttribute('aria-valuenow'),
    ).toBe('0');
    expect(hk.querySelector('.probe-card-footer')?.textContent).toBe(
      'edge-hkTraffic quota3.00 GB / Unlimited',
    );

    expect(within(uk).getByText('Not linked')).toBeTruthy();
    expect(within(uk).queryByText('Local panel')).toBeNull();
    // The usage of a server that is not online is not known: no bar at 0 of 850 GB.
    expect(within(uk).queryAllByRole('progressbar')).toHaveLength(0);
    expect(uk.querySelector('.probe-card-subtitle')).toBeNull();

    expect(statistic('Servers online')).toBe('2 / 3');
    expect(statistic('Linked to this panel')).toBe('2');
    expect(statistic('Upload speed')).toBe('3.00 KB/s');
    expect(statistic('Download speed')).toBe('5.00 KB/s');

    const link = screen.getByRole('link', { name: /Open public page$/ });
    expect(link.getAttribute('href')).toBe('https://probe.example.com');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('says so when Lite answers with no server at all', async () => {
    serve(() => new Msg(true, '', overview({ servers: [] })));
    renderWithProviders(<ProbePage />);

    expect(await screen.findByText('The Lite monitor lists no servers yet.')).toBeTruthy();
    expect(statistic('Servers online')).toBe('0 / 0');
    expect(screen.queryByText('The Lite monitor could not be read')).toBeNull();
  });

  it('reports a failed request with its message, and asks again on Refresh', async () => {
    let failing = true;
    serve(() =>
      failing
        ? new Msg(false, 'Something went wrong (database is locked)')
        : new Msg(true, '', overview()),
    );
    renderWithProviders(<ProbePage />);

    await screen.findByText('Something went wrong (database is locked)');
    expect(cards()).toHaveLength(0);

    failing = false;
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(cards()).toHaveLength(3));
  });

  // A tab left open across an upgrade may be sent a status its code has never heard of.
  it('reports an answer it does not understand instead of drawing it', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const rebooting = { ...losAngeles, status: 'rebooting' };
    serve(() => new Msg(true, '', { ...overview(), servers: [rebooting] }));
    renderWithProviders(<ProbePage />);

    expect(await screen.findByText('probe/servers response failed validation')).toBeTruthy();
    expect(cards()).toHaveLength(0);
  });

  // The page polls every 3 s: one dropped request must not blank a screen of servers.
  it('keeps the servers when a later poll fails, marked with the time they are from', async () => {
    let failing = false;
    serve(() => (failing ? new Msg(false, 'Request failed') : new Msg(true, '', overview())));
    const queryClient = makeTestQueryClient();
    renderWithProviders(<ProbePage />, { queryClient });
    await waitFor(() => expect(cards()).toHaveLength(3));

    failing = true;
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: keys.probe.servers() });
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/Showing data from \d{2}:\d{2}:\d{2}/);
    expect(alert.textContent).toContain('Request failed');
    expect(cards()).toHaveLength(3);
  });

  // The figures are live: a page that asked once would show the load of minutes ago.
  it('asks again every three seconds, but not while the tab is hidden', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const get = serve(() => new Msg(true, '', overview()));
    renderWithProviders(<ProbePage />);
    await waitFor(() => expect(cards()).toHaveLength(3));
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
    const first = renderWithProviders(<ProbePage />, { queryClient });
    await waitFor(() => expect(cards()).toHaveLength(3));
    first.unmount();

    renderWithProviders(<ProbePage />, { queryClient });
    await waitFor(() => expect(pollsOf(get)).toBe(2));
  });

  it.each([
    ['Link hosts', /Link hosts$/],
    ['Probe settings', /Settings$/],
  ])('closes the "%s" modal when the admin cancels it', async (title, button) => {
    serve(() => new Msg(true, '', overview()));
    renderWithProviders(<ProbePage />);
    await waitFor(() => expect(cards()).toHaveLength(3));

    fireEvent.click(screen.getByRole('button', { name: button }));
    const dialog = await screen.findByRole('dialog', { name: title });
    fireEvent.click(await within(dialog).findByRole('button', { name: 'Cancel' }));
    expect(isClosing(dialog)).toBe(true);
  });

  // Without the refresh the page stays on "not set up" until the next poll.
  it('shows the servers as soon as the Lite address is saved', async () => {
    let configured = false;
    serve(() => new Msg(true, '', configured ? overview() : notConfigured));
    vi.spyOn(HttpUtil, 'post').mockImplementation(async () => {
      configured = true;
      return new Msg(true, '', { url: 'http://127.0.0.1:27777', publicUrl: '' });
    });
    renderWithProviders(<ProbePage />);

    await screen.findByText('The probe is not set up yet');
    fireEvent.click(screen.getAllByRole('button', { name: /Settings$/ })[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Probe settings' });
    fireEvent.change(await within(dialog).findByLabelText('Lite address'), {
      target: { value: 'http://127.0.0.1:27777' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(cards()).toHaveLength(3));
    expect(await screen.findByText('Probe settings saved')).toBeTruthy();
    expect(isClosing(dialog)).toBe(true);
  });

  // Linking is what makes a server show up in a client's portal: the tag must follow at once.
  it('names the linked host on the card as soon as the links are saved', async () => {
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
    vi.spyOn(HttpUtil, 'post').mockImplementation(async () => {
      linked = true;
      return new Msg(true, '', links);
    });
    renderWithProviders(<ProbePage />);
    await waitFor(() => expect(cards()).toHaveLength(2));
    expect(within(cards()[1]).getByText('Not linked')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: /Link hosts$/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Link hosts' });
    await within(dialog).findByText('203.0.113.7');
    chooseSelectOption('probe-link-2', '🇭🇰 香港-Bravo');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(within(cards()[1]).getByText('edge-hk')).toBeTruthy());
    expect(await screen.findByText('Links saved')).toBeTruthy();
    expect(isClosing(dialog)).toBe(true);
  });
});
