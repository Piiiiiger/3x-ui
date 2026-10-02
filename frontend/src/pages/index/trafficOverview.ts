import { SizeFormatter } from '@/utils';

export interface Speed {
  up: number;
  down: number;
}

interface HostReport {
  id: number;
  status?: string;
  netUp?: number;
  netDown?: number;
}

/** Live speed per host (node id 0 is the panel itself) from the hosts' own reports, and the sum. */
export function liveSpeeds(
  hosts: { nodeId: number }[],
  nodes: HostReport[],
  local: Speed | null,
): { byHost: Map<number, Speed>; total: Speed } {
  const reports = new Map(nodes.map((n) => [n.id, n]));
  const byHost = new Map<number, Speed>();
  const total = { up: 0, down: 0 };
  for (const { nodeId } of hosts) {
    let speed: Speed = { up: 0, down: 0 };
    if (nodeId === 0) {
      speed = local ?? speed;
    } else {
      const report = reports.get(nodeId);
      if (report?.status === 'online') speed = { up: report.netUp ?? 0, down: report.netDown ?? 0 };
    }
    byHost.set(nodeId, speed);
    total.up += speed.up;
    total.down += speed.down;
  }
  return { byHost, total };
}

export function sizeParts(bytes: number): [string, string] {
  const [value, unit = ''] = SizeFormatter.sizeFormat(bytes).split(' ');
  return [value, unit];
}
