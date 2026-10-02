import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import ProbeServerCard, {
  ProbeMeter,
  type ProbeCardServer,
} from '@/components/probe/ProbeServerCard';

const GIB = 1024 ** 3;
const NOON_UTC = Date.UTC(2026, 9, 2, 12, 0, 0);

function online(overrides: Partial<ProbeCardServer> = {}): ProbeCardServer {
  return {
    name: '新加坡-Delta',
    region: '🇸🇬',
    status: 'online',
    updatedAt: NOON_UTC,
    cpu: 12.5,
    memUsed: 0.8 * GIB,
    memTotal: 2 * GIB,
    diskUsed: 8 * GIB,
    diskTotal: 40 * GIB,
    load1: 0.31,
    load5: 0.22,
    load15: 0.18,
    netIn: 5678,
    netOut: 1234,
    netTotalUp: 1_000_000,
    netTotalDown: 2_000_000,
    uptime: 27 * 3600,
    pings: [],
    ...overrides,
  };
}

// What the API sends for a server that is not online: every figure is zero.
function silent(status: ProbeCardServer['status'], updatedAt = 0): ProbeCardServer {
  return {
    name: '香港-Echo',
    region: '🇭🇰',
    status,
    updatedAt,
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
    pings: [],
  };
}

function cardText(): string {
  return document.querySelector('.probe-card')?.textContent ?? '';
}

function bars(): HTMLElement[] {
  return screen.queryAllByRole('progressbar');
}

function lossBars(): HTMLElement[] {
  return screen.queryAllByRole('img', { name: /Packet loss/ });
}

describe('ProbeServerCard', () => {
  it('draws an online server with named usage bars, load, uptime, speeds and totals', () => {
    render(<ProbeServerCard server={online()} />);

    expect(screen.getByText('新加坡-Delta').getAttribute('dir')).toBe('auto');
    expect(screen.getByText('🇸🇬')).toBeTruthy();
    expect(screen.getByText('Online')).toBeTruthy();

    expect(bars().map((bar) => bar.getAttribute('aria-label'))).toEqual([
      'CPU 12.5 %',
      'RAM 40.0 %',
      'Storage 20.0 %',
    ]);
    expect(screen.getByText('819.20 MB / 2.00 GB')).toBeTruthy();
    expect(screen.getByText('8.00 GB / 40.00 GB')).toBeTruthy();

    expect(screen.getByText('0.31 / 0.22 / 0.18')).toBeTruthy();
    expect(screen.getByText('1d 3h')).toBeTruthy();
    // Lite's net_out is the upload rate and net_in the download rate.
    expect(screen.getByText('↑ 1.21 KB/s')).toBeTruthy();
    expect(screen.getByText('↓ 5.54 KB/s')).toBeTruthy();
    expect(screen.getByText('Total up / down')).toBeTruthy();
    expect(screen.getByText('↑ 976.56 KB')).toBeTruthy();
    expect(screen.getByText('↓ 1.91 MB')).toBeTruthy();
  });

  // Rounded, the last half hour of every day of uptime read "28d 24h".
  it.each([
    [2504912, '28d 23h'],
    [86399, '23h'],
    [3599, '59m'],
  ])('counts only the whole units of an uptime of %i s: %s', (uptime, shown) => {
    render(<ProbeServerCard server={online({ uptime })} />);

    expect(screen.getByText('Uptime').nextElementSibling?.textContent).toBe(shown);
  });

  // A dead server must not look alive: no bars at 0 % under a red label.
  it('shows when an offline server was last seen, and none of its zeroed figures', () => {
    render(<ProbeServerCard server={silent('offline', NOON_UTC)} />);

    expect(screen.getByText('Offline')).toBeTruthy();
    // Noon UTC is the 2nd or the 3rd of October in every time zone.
    expect(screen.getByText(/^Last seen 10\/0[23]\/2026, \d{2}:\d{2}:\d{2}$/)).toBeTruthy();
    expect(bars()).toHaveLength(0);
    expect(cardText()).not.toContain('Uptime');
    expect(cardText()).not.toContain('0 B');
  });

  // The API sends updatedAt 0 for a report without a time; it must not read as 1970.
  it('leaves the last-seen line out when the offline report carries no time', () => {
    render(<ProbeServerCard server={silent('offline')} />);

    expect(screen.getByText('Offline')).toBeTruthy();
    expect(cardText()).not.toContain('Last seen');
  });

  // Nothing reported since the monitor started is not an outage.
  it('says there is no data yet for a server that has not reported', () => {
    render(<ProbeServerCard server={silent('unknown')} />);

    expect(screen.getByText('No data yet')).toBeTruthy();
    expect(screen.queryByText('Offline')).toBeNull();
    expect(bars()).toHaveLength(0);
    expect(cardText()).not.toContain('Last seen');
    expect(cardText()).not.toContain('0 B');
  });

  it('says a host the monitor does not watch is not monitored', () => {
    render(<ProbeServerCard server={silent('unmonitored')} />);

    expect(screen.getByText('Not monitored')).toBeTruthy();
    expect(screen.queryByText('No data yet')).toBeNull();
    expect(bars()).toHaveLength(0);
    expect(cardText()).not.toContain('Last seen');
    expect(cardText()).not.toContain('0 B');
  });

  // The tier colours fail text contrast on the light card, so they may only fill a bar.
  it('puts the warning and critical colours on the bars and never on text', () => {
    render(
      <ProbeServerCard server={online({ cpu: 50, memUsed: 1.7 * GIB, diskUsed: 38 * GIB })} />,
    );
    const fillOf = (name: string) =>
      screen.getByRole('progressbar', { name }).querySelector<HTMLElement>('.ant-progress-track')
        ?.style.background;
    const warn = 'rgb(250, 173, 20)';
    const critical = 'rgb(255, 77, 79)';

    expect(fillOf('RAM 85.0 %')).toBe(warn);
    expect(fillOf('Storage 95.0 %')).toBe(critical);
    expect([warn, critical]).not.toContain(fillOf('CPU 50.0 %'));

    const tinted = Array.from(document.querySelectorAll<HTMLElement>('.probe-card [style]')).filter(
      (el) => [warn, critical].some((colour) => (el.getAttribute('style') ?? '').includes(colour)),
    );
    expect(tinted.map((el) => el.className)).toEqual(['ant-progress-track', 'ant-progress-track']);
  });

  // A server that has not sent its totals yet divided by zero.
  it('reads 0 % for a total of zero instead of NaN', () => {
    render(<ProbeServerCard server={online({ memUsed: 0, memTotal: 0, diskTotal: 0 })} />);

    expect(
      screen.getByRole('progressbar', { name: 'RAM 0.0 %' }).getAttribute('aria-valuenow'),
    ).toBe('0');
    expect(
      screen.getByRole('progressbar', { name: 'Storage 0.0 %' }).getAttribute('aria-valuenow'),
    ).toBe('0');
    expect(cardText()).not.toContain('NaN');
  });

  // Lite reports a route with every sample lost as latency -1; it must not read "-1 ms".
  it('lists each ping route with its latency under one heading, and a dead route as a timeout', () => {
    render(
      <ProbeServerCard
        server={online({
          pings: [
            { id: 8, name: '电信', latency: 31, loss: 0.4 },
            { id: 9, name: '联通', latency: -1, loss: 100 },
          ],
        })}
      />,
    );
    const routes = Array.from(document.querySelectorAll('.probe-card-route')).map((route) => [
      route.querySelector('.probe-card-route-name')?.textContent,
      route.querySelector('.probe-card-route-latency')?.textContent,
    ]);

    expect(screen.getByText('Network quality')).toBeTruthy();
    expect(screen.getByText('Last hour')).toBeTruthy();
    expect(routes).toEqual([
      ['电信', '31 ms'],
      ['联通', 'timeout'],
    ]);
    // The loss has no figure of its own on the card, so the bar has to say it.
    expect(lossBars().map((bar) => bar.getAttribute('aria-label'))).toEqual([
      '电信 Packet loss 0.4 %',
      '联通 Packet loss 100 %',
    ]);
  });

  it('leaves the network quality heading out for a server without ping tasks', () => {
    render(<ProbeServerCard server={online()} />);

    expect(screen.queryByText('Network quality')).toBeNull();
    expect(lossBars()).toHaveLength(0);
  });

  // The bar is the share of the hour's pings that came back: full when none was lost.
  it('fills a route bar with the pings answered, amber under 95 % and red under 80 %', () => {
    render(
      <ProbeServerCard
        server={online({
          pings: [
            { id: 1, name: 'a', latency: 20, loss: 5 },
            { id: 2, name: 'b', latency: 20, loss: 5.5 },
            { id: 3, name: 'c', latency: 20, loss: 20 },
            { id: 4, name: 'd', latency: 20, loss: 20.5 },
            { id: 5, name: 'e', latency: -1, loss: 100 },
          ],
        })}
      />,
    );
    const fills = lossBars().map((bar) => bar.firstElementChild as HTMLElement);
    const warn = 'rgb(250, 173, 20)';
    const critical = 'rgb(255, 77, 79)';

    expect(fills.map((fill) => fill.style.width)).toEqual(['95%', '94.5%', '80%', '79.5%', '0%']);
    expect(fills.slice(1, 4).map((fill) => fill.style.background)).toEqual([warn, warn, critical]);
    expect([warn, critical]).not.toContain(fills[0].style.background);
  });

  // Lite's own thresholds, so a route has the same colour here as on Lite's page.
  it.each([
    [80, 'is-good'],
    [81, 'is-warn'],
    [179, 'is-warn'],
    [180, 'is-bad'],
    [-1, 'is-bad'],
  ])('marks a route latency of %i ms as %s', (latency, tone) => {
    render(
      <ProbeServerCard
        server={online({
          pings: [{ id: 8, name: '电信', latency, loss: latency < 0 ? 100 : 0 }],
        })}
      />,
    );

    expect(document.querySelector('.probe-card-route-latency')?.className).toBe(
      `probe-card-route-latency ${tone}`,
    );
  });

  // Both are cut to one line so that cards line up; the tooltip keeps the rest in reach.
  it('carries the whole name and system line as tooltips', () => {
    const system = 'Debian GNU/Linux 13 (trixie) · amd64 · kvm · 1-core';
    render(<ProbeServerCard server={online()} subtitle={system} />);

    expect(screen.getByText('新加坡-Delta').getAttribute('title')).toBe('新加坡-Delta');
    expect(screen.getByText(system).getAttribute('title')).toBe(system);
  });

  // A quota without a limit has nothing to divide by: no figure, and no bar at "0 %".
  it('draws a meter without a percentage as an empty bar named by its detail', () => {
    render(<ProbeMeter label="Traffic quota" percent={null} detail="3.00 GB / Unlimited" />);
    const bar = screen.getByRole('progressbar', { name: 'Traffic quota 3.00 GB / Unlimited' });

    expect(bar.getAttribute('aria-valuenow')).toBe('0');
    expect(document.querySelector('.probe-card-meter')?.textContent).toBe(
      'Traffic quota3.00 GB / Unlimited',
    );
  });

  // Lite hands a region set by hand through as a two-letter code, not as a flag.
  it('draws the flag of a region given as a country code', () => {
    render(<ProbeServerCard server={online({ region: 'sg' })} />);

    expect(screen.getByText('🇸🇬')).toBeTruthy();
    expect(cardText()).not.toContain('sg');
  });
});
