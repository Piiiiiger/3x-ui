import type { Inbound } from '@/schemas/api/inbound';
import type { ExternalProxyEntry } from '@/schemas/protocols/stream/external-proxy';

import { resolveAddr, resolveShareHost, type ShareHostFields } from './inbound-link';

/** What a client dials: an address and the port links advertise for it. */
export interface PublicEndpoint {
  dest: string;
  port: number;
}

type PortFields = ShareHostFields & { port?: number; sharePort?: number };

// A public port only counts when NAT maps a different one onto the inbound.
function publicPortOf(fields: PortFields): number {
  const sharePort = fields.sharePort ?? 0;
  return sharePort > 0 && sharePort !== (fields.port ?? 0) ? sharePort : 0;
}

// Behind NAT, links carry the public port through the one externalProxy entry the
// subscription synthesizes (withPublicPort in internal/sub); own entries win.
export function withPublicPort(
  inbound: Inbound,
  hostOverride: string,
  fallbackHostname: string,
): Inbound {
  const port = publicPortOf(inbound);
  const own = inbound.streamSettings?.externalProxy ?? [];
  if (port === 0 || own.length > 0) return inbound;
  const entry: ExternalProxyEntry = {
    forceTls: 'same',
    dest: resolveAddr(inbound, hostOverride, fallbackHostname),
    port,
    remark: '',
  };
  return {
    ...inbound,
    streamSettings: { ...inbound.streamSettings, externalProxy: [entry] },
  } as Inbound;
}

/** The endpoint links advertise for an inbound: its share address and public port. */
export function advertisedEndpoint(
  fields: PortFields,
  hostOverride: string,
  fallbackHostname: string,
): PublicEndpoint {
  return {
    dest: resolveShareHost(fields, hostOverride, fallbackHostname),
    port: publicPortOf(fields) || (fields.port ?? 0),
  };
}

/** The endpoint behind NAT, or undefined when clients dial the inbound's own port. */
export function publicEndpointOf(
  fields: PortFields,
  hostOverride: string,
  fallbackHostname: string,
): PublicEndpoint | undefined {
  return publicPortOf(fields) === 0
    ? undefined
    : advertisedEndpoint(fields, hostOverride, fallbackHostname);
}

/** dest:port as people type it, an IPv6 literal bracketed. */
export function endpointLabel({ dest, port }: PublicEndpoint): string {
  return dest.includes(':') && !dest.startsWith('[') ? `[${dest}]:${port}` : `${dest}:${port}`;
}
