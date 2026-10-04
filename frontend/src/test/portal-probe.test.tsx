import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { keys } from '@/api/queryKeys';
import type { PortalProbe, PortalProbeServer } from '@/generated/zod';
import PortalApp from '@/pages/sub/portal/PortalApp';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

// The usage chart draws on a canvas, which jsdom does not have.
vi.mock('uplot', () => ({
  default: class {
    static paths = { spline: () => undefined };
    static pxRatio = 1;
    setData() {}
    setSize() {}
    redraw() {}
    destroy() {}
  },
}));

const BASE = '/x/portal';
const NOON_UTC = Date.UTC(2026, 9, 2, 12, 0, 0);

function host(overrides: Partial<PortalProbeServer>): PortalProbeServer {
  return {
    id: 0,
    name: 'host',
    status: 'online',
    region: '🇭🇰',
    updatedAt: NOON_UTC,
    cpu: 12.5,
    memUsed: 1,
    memTotal: 2,
    diskUsed: 1,
    diskTotal: 4,
    load1: 0.1,
    load5: 0.1,
    load15: 0.1,
    netIn: 10,
    netOut: 10,
    netTotalUp: 100,
    netTotalDown: 100,
    uptime: 3600,
    pings: [],
    ...overrides,
  };
}

function probeOf(servers: PortalProbeServer[], overrides: Partial<PortalProbe> = {}): PortalProbe {
  return { enabled: true, fetchedAt: NOON_UTC, stale: false, servers, ...overrides };
}

// 'absent' is the answer of a server older than the probe view: it sends no flag.
function portalData(email: string, probe: boolean | 'absent' = true) {
  return {
    email,
    page: { sId: `sub-${email}`, emails: [email], enabled: true },
    plan: {
      name: 'Standard',
      totalGB: 0,
      durationDays: 30,
      trafficReset: 'never',
      trafficResetDay: 1,
      limitIp: 0,
    },
    daily: [],
    ...(probe === 'absent' ? {} : { probe }),
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const unauthorized = () => json({ error: 'unauthorized' }, 401);

// What the portal's server would answer; each test rewrites the parts it needs.
interface Server {
  data: () => Response | Promise<Response>;
  probe: () => Response | Promise<Response>;
  login: () => Response;
  logout: () => Response;
}

function serve(server: Partial<Server>) {
  const routes: Server = {
    data: unauthorized,
    probe: unauthorized,
    login: () => json({ success: true }),
    logout: () => json({ success: true }),
    ...server,
  };
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const name = url.slice(BASE.length + 1) as keyof Server;
      calls.push(name);
      if (!(name in routes)) throw new Error(`unexpected request ${url}`);
      return routes[name]();
    }),
  );
  return { routes, countOf: (name: keyof Server) => calls.filter((call) => call === name).length };
}

function cardNames(): (string | null)[] {
  return Array.from(document.querySelectorAll('.probe-card-name')).map((el) => el.textContent);
}

async function openProbeView() {
  fireEvent.click(await screen.findByRole('radio', { name: 'Probe' }));
}

beforeEach(() => {
  window.history.replaceState(null, '', '/x/portal');
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('the portal probe view', () => {
  // The switch would only ever lead a client without a monitored server to "Not monitored".
  it('offers no switch to a client the server gives no probe view', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    const server = serve({ data: () => json(portalData('alice', false)) });
    renderWithProviders(<PortalApp base={BASE} />);

    expect(await screen.findByText('My plan')).toBeTruthy();
    expect(screen.queryByRole('radio', { name: 'Probe' })).toBeNull();
    expect(screen.getByRole('radio', { name: '自定义订阅' })).toBeTruthy();
    expect(server.countOf('probe')).toBe(0);
  });

  // The portal data is parsed strictly: an answer without the new field must still open it.
  it('still opens against a server that does not know the probe view', async () => {
    serve({ data: () => json(portalData('alice', 'absent')) });
    renderWithProviders(<PortalApp base={BASE} />);

    expect(await screen.findByText('My plan')).toBeTruthy();
    expect(screen.queryByRole('radio', { name: 'Probe' })).toBeNull();
    expect(screen.getByRole('radio', { name: '自定义订阅' })).toBeTruthy();
    expect(screen.queryByText('Your details could not be loaded. Try again later.')).toBeNull();
  });

  it('switches between the overview and the servers of the client, and keeps the view in the address', async () => {
    const server = serve({
      data: () => json(portalData('alice')),
      probe: () =>
        json(
          probeOf([
            host({ id: 0, name: 'Hong Kong Backup', provider: 'Azure' }),
            host({ id: 2, name: 'Tokyo', status: 'unmonitored', region: '', updatedAt: 0 }),
          ]),
        ),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    expect(await screen.findByText('My plan')).toBeTruthy();
    expect(server.countOf('probe')).toBe(0);

    await openProbeView();
    await waitFor(() => expect(cardNames()).toEqual(['Hong Kong Backup', 'Tokyo']));
    expect(screen.getByText('Azure')).toBeTruthy();
    expect(screen.getByText('Not monitored')).toBeTruthy();
    expect(screen.queryByText('My plan')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(window.location.hash).toBe('#probe');

    fireEvent.click(screen.getByRole('radio', { name: 'Overview' }));
    expect(await screen.findByText('My plan')).toBeTruthy();
    expect(cardNames()).toEqual([]);
    expect(window.location.href).toBe(`${window.location.origin}/x/portal`);
  });

  it('opens on the probe view when the address names it, as after a reload', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    serve({
      data: () => json(portalData('alice')),
      probe: () => json(probeOf([host({ name: 'Hong Kong' })])),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    await waitFor(() => expect(cardNames()).toEqual(['Hong Kong']));
    expect(screen.queryByText('My plan')).toBeNull();
  });

  // The figures are live; and a tab nobody looks at must not keep the monitor busy.
  it('asks again every five seconds, but not while the tab is hidden', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    window.history.replaceState(null, '', '/x/portal#probe');
    const server = serve({
      data: () => json(portalData('alice')),
      probe: () => json(probeOf([host({ name: 'Hong Kong' })])),
    });
    renderWithProviders(<PortalApp base={BASE} />);
    await waitFor(() => expect(cardNames()).toEqual(['Hong Kong']));
    expect(server.countOf('probe')).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(server.countOf('probe')).toBe(2);

    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });
    expect(server.countOf('probe')).toBe(2);
  });

  it('says from when the figures are while the monitor does not answer', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    serve({
      data: () => json(portalData('alice')),
      probe: () => json(probeOf([host({ name: 'Hong Kong' })], { stale: true })),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/^Showing data from \d{2}:\d{2}:\d{2}$/);
    expect(cardNames()).toEqual(['Hong Kong']);
  });

  // With nothing usable the server still lists the client's hosts, without figures or a time.
  it('says monitoring is unavailable when there are no figures to date', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    serve({
      data: () => json(portalData('alice')),
      probe: () =>
        json(
          probeOf([host({ name: 'Hong Kong', status: 'unknown', updatedAt: 0 })], {
            stale: true,
            fetchedAt: 0,
          }),
        ),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Monitoring is temporarily unavailable');
    expect(cardNames()).toEqual(['Hong Kong']);
    expect(screen.getByText('No data yet')).toBeTruthy();
  });

  // The view polls every 5 s: one dropped request must not blank the client's servers.
  it('keeps the last figures on screen when a later poll fails', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    const server = serve({
      data: () => json(portalData('alice')),
      probe: () => json(probeOf([host({ name: 'Hong Kong' })])),
    });
    const queryClient = makeTestQueryClient();
    renderWithProviders(<PortalApp base={BASE} />, { queryClient });
    await waitFor(() => expect(cardNames()).toEqual(['Hong Kong']));

    server.routes.probe = () => json({ error: 'server' }, 500);
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: keys.portal.probes() });
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/^Showing data from \d{2}:\d{2}:\d{2}$/);
    expect(cardNames()).toEqual(['Hong Kong']);
  });

  it('says monitoring is unavailable when the first request fails', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    serve({
      data: () => json(portalData('alice')),
      probe: () => json({ error: 'server' }, 500),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Monitoring is temporarily unavailable');
    expect(cardNames()).toEqual([]);
  });

  // A tab left open across an upgrade may be sent a status its code has never heard of.
  it('reports an answer it does not understand instead of drawing it', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    const rebooting = { ...host({ name: 'Hong Kong' }), status: 'rebooting' };
    serve({
      data: () => json(portalData('alice')),
      probe: () => json({ ...probeOf([]), servers: [rebooting] }),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Monitoring is temporarily unavailable');
    expect(cardNames()).toEqual([]);
  });

  // The probe flag is read once at sign-in; the list can be empty by the time it is asked for.
  it('says so when there is no server to show', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    serve({
      data: () => json(portalData('alice')),
      probe: () => json(probeOf([], { enabled: false, fetchedAt: 0 })),
    });
    renderWithProviders(<PortalApp base={BASE} />);

    expect(await screen.findByText('No server status to show yet.')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  // A session that ended (expired, password changed, signed out elsewhere) must lead
  // to the sign-in form, and must not be asked for figures again meanwhile.
  it('asks who is signed in when the session has ended, and shows the sign-in form', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    window.history.replaceState(null, '', '/x/portal#probe');
    let signedIn = true;
    let answerData: (response: Response) => void = () => {};
    const server = serve({
      data: () =>
        signedIn
          ? json(portalData('alice'))
          : new Promise<Response>((resolve) => {
              answerData = resolve;
            }),
      probe: () => (signedIn ? json(probeOf([host({ name: 'Hong Kong' })])) : unauthorized()),
    });
    // The panel's client retries a failed request once; a 401 must not be asked twice.
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1 } } });
    renderWithProviders(<PortalApp base={BASE} />, { queryClient });
    await waitFor(() => expect(cardNames()).toEqual(['Hong Kong']));

    signedIn = false;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    await waitFor(() => expect(server.countOf('data')).toBe(2));
    expect(cardNames()).toEqual([]);

    // The answer about the session is slow: two more polls would have been due.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(server.countOf('probe')).toBe(2);
    expect(server.countOf('data')).toBe(2);

    await act(async () => {
      answerData(unauthorized());
    });
    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeTruthy();
  });

  // One browser, two people: the second must never be shown the servers of the first.
  it('shows the next client nothing of the previous one when their own figures fail to load', async () => {
    let who: 'alice' | 'bob' | null = 'alice';
    const server = serve({
      data: () => (who ? json(portalData(who)) : unauthorized()),
      probe: () => {
        if (who === 'alice') return json(probeOf([host({ name: 'Alice Tokyo' })]));
        throw new TypeError('Failed to fetch');
      },
      login: () => {
        who = 'bob';
        return json({ success: true });
      },
      logout: () => {
        who = null;
        return json({ success: true });
      },
    });
    // The panel's client: answers stay in the cache for five minutes by default.
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderWithProviders(<PortalApp base={BASE} />, { queryClient });
    await openProbeView();
    await waitFor(() => expect(cardNames()).toEqual(['Alice Tokyo']));

    fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    fireEvent.change(await screen.findByLabelText('Username'), { target: { value: 'bob' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Monitoring is temporarily unavailable');
    expect(server.countOf('probe')).toBeGreaterThanOrEqual(2);
    expect(document.body.textContent).not.toContain('Alice Tokyo');
    expect(queryClient.getQueryData(keys.portal.probe(BASE, 'alice'))).toBeUndefined();
  });

  // Another tab can sign someone else in under an open portal: the cookie is shared.
  it('drops the figures of one client when the portal turns out to belong to another', async () => {
    window.history.replaceState(null, '', '/x/portal#probe');
    let who = 'alice';
    serve({
      data: () => json(portalData(who)),
      probe: () => {
        if (who === 'alice') return json(probeOf([host({ name: 'Alice Tokyo' })]));
        throw new TypeError('Failed to fetch');
      },
    });
    const queryClient = makeTestQueryClient();
    renderWithProviders(<PortalApp base={BASE} />, { queryClient });
    await waitFor(() => expect(cardNames()).toEqual(['Alice Tokyo']));

    who = 'bob';
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: keys.portal.data(BASE) });
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Monitoring is temporarily unavailable');
    expect(document.body.textContent).not.toContain('Alice Tokyo');
  });
});
