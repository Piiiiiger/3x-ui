import type { PlanSummary } from '@/generated/zod';
import { hostEndpointsFor } from '@/lib/hosts/host-link';
import type { HostRecord } from '@/schemas/api/host';
import type { InboundOption } from '@/schemas/client';

/** A new node's name: the base itself while free, else base-2, base-3 and so on. */
export function uniqueNodeName(base: string, taken: Iterable<string>): string {
  const used = new Set(taken);
  if (!used.has(base)) return base;
  for (let n = 2; ; n++) {
    if (!used.has(`${base}-${n}`)) return `${base}-${n}`;
  }
}

/** The VLESS REALITY node a generated one copies: one on the same host, else any. */
export function pickTemplate(options: InboundOption[], host: number): InboundOption | undefined {
  const reality = options.filter((o) => o.protocol === 'vless' && o.security === 'reality');
  return reality.find((o) => (o.nodeId ?? 0) === host) ?? reality[0];
}

/** Plans that already give their members every node of the host; none for an empty host. */
export function pretickedPlans(plans: PlanSummary[], hostNodeIds: number[]): number[] {
  if (hostNodeIds.length === 0) return [];
  return plans.filter((p) => hostNodeIds.every((id) => p.inboundIds.includes(id))).map((p) => p.id);
}

/** Whether an enabled entry moves one of the host's nodes to another public port, as NAT does. */
export function isNatHost(
  hostNodes: { id: number; port: number }[],
  entries: HostRecord[],
): boolean {
  return hostNodes.some((node) =>
    hostEndpointsFor(entries, node.id, node.port, '').some(
      (endpoint) => endpoint.port !== node.port,
    ),
  );
}

/** The copied stream settings with the chosen REALITY target and server name. */
export function withRealityTarget(
  streamSettings: string,
  target: string,
  serverName: string,
): string {
  const stream = JSON.parse(streamSettings) as Record<string, unknown>;
  const reality = (stream.realitySettings as Record<string, unknown> | undefined) ?? {};
  return JSON.stringify({
    ...stream,
    realitySettings: { ...reality, target, serverNames: serverName ? [serverName] : [] },
  });
}
