// Why the IP limit ignores an address; '' means the address counts.
export type ClientIpExempt = '' | 'host' | 'allowlist' | 'private';

// One entry of POST /panel/api/clients/ips/:email; node '' is this panel, and
// bannedUntil is unix seconds (0 = not banned).
export type ClientIpInfo = {
  ip: string;
  time: string;
  node: string;
  exempt: ClientIpExempt;
  exemptHost: string;
  bannedUntil: number;
};

// A running ban from POST /panel/api/clients/ipBans/:email; network is an IPv4
// address or an IPv6 /64.
export type ClientIpBan = {
  network: string;
  bannedAt: number;
  expiresAt: number;
};

const EXEMPT_KINDS: readonly ClientIpExempt[] = ['host', 'allowlist', 'private'];

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

function num(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

// Also accepts the legacy shape (a plain array of "ip (time)" strings) so the
// UI keeps working against older panels.
export function normalizeClientIps(obj: unknown): ClientIpInfo[] {
  if (!Array.isArray(obj)) return [];
  const out: ClientIpInfo[] = [];
  for (const x of obj) {
    if (typeof x === 'string') {
      if (x.length > 0)
        out.push({ ip: x, time: '', node: '', exempt: '', exemptHost: '', bannedUntil: 0 });
      continue;
    }
    if (x && typeof x === 'object') {
      const o = x as Record<string, unknown>;
      const ip = str(o.ip);
      if (!ip) continue;
      const exempt = EXEMPT_KINDS.find((k) => k === o.exempt) ?? '';
      out.push({
        ip,
        time: str(o.time),
        node: str(o.node),
        exempt,
        exemptHost: str(o.exemptHost),
        bannedUntil: num(o.bannedUntil),
      });
    }
  }
  return out;
}

export function normalizeClientIpBans(obj: unknown): ClientIpBan[] {
  if (!Array.isArray(obj)) return [];
  const out: ClientIpBan[] = [];
  for (const x of obj) {
    if (!x || typeof x !== 'object') continue;
    const o = x as Record<string, unknown>;
    const network = str(o.network);
    if (network) out.push({ network, bannedAt: num(o.bannedAt), expiresAt: num(o.expiresAt) });
  }
  return out;
}

// Minutes left on a ban, rounded up so a running ban never reads as 0.
export function banMinutesLeft(expiresAt: number, nowMs: number): number {
  return Math.max(1, Math.ceil((expiresAt * 1000 - nowMs) / 60_000));
}
