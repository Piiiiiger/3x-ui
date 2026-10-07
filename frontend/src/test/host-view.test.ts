import { describe, expect, it } from 'vitest';

import type { ProbeServer } from '@/generated/zod';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { Status } from '@/models/status';
import { localHostView, remoteHostView } from '@/pages/nodes/hostView';

const GIB = 1024 ** 3;

const node: NodeRecord = {
  id: 7,
  name: 'edge',
  kind: 'agent',
  enable: true,
  status: 'online',
  address: '192.0.2.7',
  cpuPct: 9,
  memPct: 40,
  uptimeSecs: 60,
};

const ping = {
  id: 1,
  name: 'CT',
  latency: 42,
  loss: 0,
  blocks: [{ start: 1, end: 2, checks: 3, loss: 0 }],
};

function probe(status: ProbeServer['status']): ProbeServer {
  return {
    id: 'uuid-edge',
    name: 'edge',
    region: '',
    os: '',
    arch: '',
    virtualization: '',
    cpuCores: 1,
    status,
    updatedAt: 0,
    cpu: 55,
    memUsed: 1 * GIB,
    memTotal: 4 * GIB,
    diskUsed: 0,
    diskTotal: 0,
    load1: 0,
    load5: 0,
    load15: 0,
    netIn: 10,
    netOut: 20,
    netTotalUp: 0,
    netTotalDown: 0,
    uptime: 999,
    trafficLimit: 0,
    trafficUsed: 3 * GIB,
    trafficResetDay: 22,
    pings: [ping],
    linked: true,
    nodeId: 7,
    nodeName: 'edge',
  };
}

describe('host views', () => {
  it("takes a linked server's live figures over the heartbeat's", () => {
    const view = remoteHostView(node, probe('online'), []);
    expect(view.cpu).toEqual({ percent: 55 });
    expect(view.mem).toEqual({ percent: 25, used: 1 * GIB, total: 4 * GIB });
    expect(view.traffic).toEqual({ used: 3 * GIB, limit: 0, resetDay: 22 });
    expect(view.uptimeSecs).toBe(999);
  });

  // Lite keeps an offline server's last hour of pings, not a load to trust.
  it('keeps the pings of an offline server but none of its figures', () => {
    const view = remoteHostView(node, probe('offline'), []);
    expect(view.pings).toEqual([ping]);
    expect(view.traffic).toBeNull();
    expect(view.speed).toBeNull();
    expect(view.cpu).toEqual({ percent: 9 });
    expect(view.mem).toEqual({ percent: 40 });
    expect(view.uptimeSecs).toBe(60);
  });

  it('marks a host without a probe server, and an Xray that stopped while it is up', () => {
    const view = remoteHostView({ ...node, xrayState: 'stop' }, undefined, []);
    expect(view.pings).toBeNull();
    expect(view.xrayIssue).toBe('stop');
    expect(
      remoteHostView({ ...node, status: 'offline', xrayState: 'stop' }, undefined, []).xrayIssue,
    ).toBe('');
  });

  it("gives this panel's host the status poll's figures and no unknown address", () => {
    const status = new Status({
      cpu: 12,
      mem: { current: 1 * GIB, total: 2 * GIB },
      publicIP: { ipv4: 'N/A', ipv6: 'N/A' },
      xray: { state: 'error', errorMsg: 'x', version: '25.1', color: 'red' },
    });
    const view = localHostView(
      status,
      undefined,
      [{ id: 1 }, { id: 2, enable: false }],
      'Local',
      'v3.1.0',
      1,
    );
    expect(view.address).toBe('');
    expect(view.cpu).toEqual({ percent: 12 });
    expect(view.mem).toEqual({ percent: 50, used: 1 * GIB, total: 2 * GIB });
    expect(view.xrayIssue).toBe('error');
    expect([view.nodesEnabled, view.nodesTotal]).toEqual([1, 2]);
  });
});
