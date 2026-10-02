import { RandomUtil } from '@/utils';
import { createDefaultInboundSettings } from '@/lib/xray/inbound-defaults';
import { coerceInboundJsonField, type DBInbound } from '@/models/dbinbound';

/*
 * Payload for POST /panel/api/inbounds/add reproducing `dbInbound` as a
 * staged copy: fresh port, empty client list (emails are unique panel-wide
 * and UUIDs must not repeat across nodes), disabled, no tag (the backend
 * regenerates one with the correct per-node prefix), cleared listen (listen
 * addresses are node-local). `nodeId === null` targets the local panel; the
 * field is omitted from the wire payload then, matching the add-form adapter.
 */
export function buildClonePayload(
  dbInbound: DBInbound,
  port: number,
  nodeId: number | null,
  share: CloneShare,
  reality?: FreshReality,
) {
  let clonedSettings: string;
  try {
    const raw = { ...coerceInboundJsonField(dbInbound.settings) };
    raw.clients = [];
    clonedSettings = JSON.stringify(raw);
  } catch {
    const fallback = createDefaultInboundSettings(dbInbound.protocol);
    clonedSettings = fallback ? JSON.stringify(fallback, null, 2) : '{}';
  }
  const sourceStream =
    typeof dbInbound.streamSettings === 'string'
      ? dbInbound.streamSettings
      : JSON.stringify(dbInbound.streamSettings ?? {});
  const streamSettingsString = reality ? withFreshReality(sourceStream, reality) : sourceStream;
  const sniffingString =
    typeof dbInbound.sniffing === 'string'
      ? dbInbound.sniffing
      : JSON.stringify(dbInbound.sniffing ?? {});
  return {
    up: 0,
    down: 0,
    total: 0,
    remark: `${dbInbound.remark} (clone)`,
    enable: false,
    expiryTime: 0,
    listen: '',
    port,
    protocol: dbInbound.protocol,
    settings: clonedSettings,
    streamSettings: streamSettingsString,
    sniffing: sniffingString,
    shareAddrStrategy: share.shareAddrStrategy,
    shareAddr: share.shareAddr,
    ...(nodeId != null ? { nodeId } : {}),
  };
}

/** The link-address settings an inbound advertises (strategy plus custom address). */
export interface CloneShare {
  shareAddrStrategy: string;
  shareAddr: string;
}

/** REALITY values a copy must not share with its source. */
export interface FreshReality {
  privateKey: string;
  publicKey: string;
  shortIds: string[];
  spiderX: string;
  mldsa65?: { seed: string; verify: string };
}

/** The link address of each host's first node in subscription order, keyed like portsInUse. */
export function hostShares(inbounds: DBInbound[]): Map<number, CloneShare> {
  const ordered = [...inbounds].sort((a, b) => a.subSortIndex - b.subSortIndex || a.id - b.id);
  const shares = new Map<number, CloneShare>();
  for (const ib of ordered) {
    const host = ib.nodeId ?? 0;
    if (!shares.has(host)) {
      shares.set(host, { shareAddrStrategy: ib.shareAddrStrategy, shareAddr: ib.shareAddr });
    }
  }
  return shares;
}

/*
 * A copy advertises its own host: the source's address on the same host, else the
 * address a node there already uses, else that host's public address.
 */
export function cloneShareFor(
  source: DBInbound,
  target: number,
  shares: Map<number, CloneShare>,
): CloneShare {
  if ((source.nodeId ?? 0) === target) {
    return { shareAddrStrategy: source.shareAddrStrategy, shareAddr: source.shareAddr };
  }
  return shares.get(target) ?? { shareAddrStrategy: 'node', shareAddr: '' };
}

function withFreshReality(streamSettings: string, fresh: FreshReality): string {
  const stream = JSON.parse(streamSettings) as Record<string, unknown>;
  const reality = stream.realitySettings as Record<string, unknown> | undefined;
  if (stream.security !== 'reality' || !reality) return streamSettings;
  const settings = (reality.settings as Record<string, unknown> | undefined) ?? {};
  return JSON.stringify({
    ...stream,
    realitySettings: {
      ...reality,
      privateKey: fresh.privateKey,
      shortIds: fresh.shortIds,
      ...('mldsa65Seed' in reality ? { mldsa65Seed: fresh.mldsa65?.seed ?? '' } : {}),
      settings: {
        ...settings,
        publicKey: fresh.publicKey,
        spiderX: fresh.spiderX,
        ...('mldsa65Verify' in settings ? { mldsa65Verify: fresh.mldsa65?.verify ?? '' } : {}),
      },
    },
  });
}

/*
 * Random clone port in the add-form's range, avoiding ports already bound on
 * the target node (client-side pre-check; the backend's node-scoped conflict
 * check stays the final arbiter). A few random tries cover the common sparse
 * case; a target so dense that those all miss falls back to a deterministic
 * scan so a free port is always found when one exists.
 */
export function pickClonePort(used: Set<number> | undefined): number {
  let port = RandomUtil.randomInteger(10000, 60000);
  if (!used) return port;
  for (let attempts = 0; attempts < 20 && used.has(port); attempts++) {
    port = RandomUtil.randomInteger(10000, 60000);
  }
  if (used.has(port)) {
    for (port = 10000; port <= 60000 && used.has(port); port++) {
      /* dense-range scan */
    }
    if (port > 60000) port = RandomUtil.randomInteger(10000, 60000);
  }
  return port;
}
