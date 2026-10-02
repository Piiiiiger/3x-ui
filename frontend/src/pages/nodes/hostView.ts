import type { ProbePing, ProbeServer } from '@/generated/zod';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { Status } from '@/models/status';

export type HostKind = 'local' | 'agent' | 'panel';

/** A share of something; used and total are known when the source reports sizes. */
export interface HostMeter {
  percent: number;
  used?: number;
  total?: number;
}

/** Everything a host card draws, whichever of the panel or the probe it came from. */
export interface HostView {
  key: string;
  /** null for this panel's own host. */
  nodeId: number | null;
  name: string;
  kind: HostKind;
  online: boolean;
  /** Reachable, but its Xray reports an error or is stopped. */
  xrayIssue: '' | 'error' | 'stop';
  enabled: boolean;
  address: string;
  nodesEnabled: number;
  nodesTotal: number;
  xrayVersion: string;
  cpu: HostMeter | null;
  mem: HostMeter | null;
  disk: HostMeter | null;
  speed: { up: number; down: number } | null;
  traffic: { used: number; limit: number } | null;
  /** null when no probe server is linked to the host. */
  pings: ProbePing[] | null;
  uptimeSecs: number;
  /** Unix seconds; 0 when the host has no heartbeat of its own. */
  lastHeartbeat: number;
  transitive: boolean;
}

/** One of a host's nodes; a node without the flag counts as enabled. */
export interface HostNodeFlag {
  id: number;
  enable?: boolean;
}

function share(used: number, total: number): HostMeter {
  return { percent: total > 0 ? (used / total) * 100 : 0, used, total };
}

function nodeCounts(nodes: HostNodeFlag[]) {
  return {
    nodesEnabled: nodes.filter((node) => node.enable !== false).length,
    nodesTotal: nodes.length,
  };
}

// Lite keeps an offline server's pings but no live figures.
function probeFigures(probe: ProbeServer | undefined) {
  const live = probe?.status === 'online' ? probe : undefined;
  return {
    cpu: live ? { percent: live.cpu } : null,
    mem: live ? share(live.memUsed, live.memTotal) : null,
    disk: live ? share(live.diskUsed, live.diskTotal) : null,
    speed: live ? { up: live.netOut, down: live.netIn } : null,
    traffic: live ? { used: live.trafficUsed, limit: live.trafficLimit } : null,
    pings: probe ? probe.pings : null,
  };
}

function xrayIssueOf(state: string | undefined): HostView['xrayIssue'] {
  const xs = (state || '').toLowerCase().trim();
  return xs === 'error' || xs === 'stop' ? xs : '';
}

export function remoteHostView(
  node: NodeRecord,
  probe: ProbeServer | undefined,
  nodes: HostNodeFlag[],
): HostView {
  const online = node.status === 'online';
  const figures = probeFigures(probe);
  // Without a probe server the heartbeat still carries the host's CPU and memory.
  const heartbeat = online && !figures.cpu;
  return {
    key: node.transitive ? `t-${node.guid || node.name}` : String(node.id),
    nodeId: node.id,
    name: node.name || '',
    kind: node.kind === 'agent' ? 'agent' : 'panel',
    online,
    xrayIssue: online ? xrayIssueOf(node.xrayState) : '',
    enabled: !!node.enable,
    address: node.address || '',
    ...nodeCounts(nodes),
    xrayVersion: node.xrayVersion || '',
    ...figures,
    cpu:
      figures.cpu ??
      (heartbeat && typeof node.cpuPct === 'number' ? { percent: node.cpuPct } : null),
    mem:
      figures.mem ??
      (heartbeat && typeof node.memPct === 'number' ? { percent: node.memPct } : null),
    uptimeSecs: probe?.status === 'online' ? probe.uptime : node.uptimeSecs || 0,
    lastHeartbeat: node.lastHeartbeat || 0,
    transitive: !!node.transitive,
  };
}

/** This panel's own host: always reachable, and its status poll fills what no probe gives. */
export function localHostView(
  status: Status,
  probe: ProbeServer | undefined,
  nodes: HostNodeFlag[],
  name: string,
): HostView {
  const figures = probeFigures(probe);
  const ip = String(status.publicIP.ipv4 || '');
  return {
    key: 'local',
    nodeId: null,
    name,
    kind: 'local',
    online: true,
    xrayIssue: xrayIssueOf(status.xray.state),
    enabled: true,
    address: ip === '0' || ip === 'N/A' ? '' : ip,
    ...nodeCounts(nodes),
    xrayVersion: status.xray.version,
    ...figures,
    cpu: figures.cpu ?? { percent: status.cpu.current },
    mem: figures.mem ?? share(status.mem.current, status.mem.total),
    disk: figures.disk ?? share(status.disk.current, status.disk.total),
    speed: figures.speed ?? { up: status.netIO.up, down: status.netIO.down },
    uptimeSecs: status.uptime,
    lastHeartbeat: 0,
    transitive: false,
  };
}
