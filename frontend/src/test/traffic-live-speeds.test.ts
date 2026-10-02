import { describe, expect, it } from 'vitest';

import { liveSpeeds } from '@/pages/index/trafficOverview';

describe('liveSpeeds', () => {
  const hosts = [{ nodeId: 0 }, { nodeId: 2 }, { nodeId: 3 }, { nodeId: 7 }];
  const nodes = [
    { id: 2, status: 'online', netUp: 100, netDown: 400 },
    // An offline host's last report is stale, so it adds nothing.
    { id: 3, status: 'offline', netUp: 9_000, netDown: 9_000 },
  ];

  it('takes the panel from its own status and each host from its last report', () => {
    const { byHost, total } = liveSpeeds(hosts, nodes, { up: 10, down: 20 });
    expect(byHost.get(0)).toEqual({ up: 10, down: 20 });
    expect(byHost.get(2)).toEqual({ up: 100, down: 400 });
    expect(byHost.get(3)).toEqual({ up: 0, down: 0 });
    expect(byHost.get(7)).toEqual({ up: 0, down: 0 });
    expect(total).toEqual({ up: 110, down: 420 });
  });

  it('counts the panel as idle until its status arrives', () => {
    const { byHost, total } = liveSpeeds(hosts, nodes, null);
    expect(byHost.get(0)).toEqual({ up: 0, down: 0 });
    expect(total).toEqual({ up: 100, down: 400 });
  });
});
