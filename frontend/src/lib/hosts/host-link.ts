import type { ExternalProxyEntry } from '@/schemas/protocols/stream/external-proxy';
import { AlpnSchema, UtlsFingerprintSchema } from '@/schemas/protocols/security/tls';
import type { HostFormValues, HostRecord } from '@/schemas/api/host';
import type { Inbound } from '@/schemas/api/inbound';
import { resolveAddr, resolveShareHost } from '@/lib/xray/inbound-link';

// The subset of a host that affects its share link. Mirrors the fields the
// backend's hostToExternalProxyMap reads.
export type HostLinkInput = Pick<
  HostFormValues,
  | 'security'
  | 'address'
  | 'port'
  | 'remark'
  | 'sni'
  | 'alpn'
  | 'fingerprint'
  | 'pinnedPeerCertSha256'
  | 'verifyPeerCertByName'
  | 'echConfigList'
  | 'overrideSniFromAddress'
  | 'keepSniBlank'
  | 'vlessRoute'
  | 'allowInsecure'
>;

// hostToExternalProxyEntry projects a host onto the ExternalProxyEntry shape the
// share-link preview generators already understand — the frontend mirror of the
// backend's hostToExternalProxyMap. security "reality"/"same" keep the inbound's
// base TLS (forceTls "same"); the preview falls back to port 443 when the host
// inherits the inbound port (port 0).
export function hostToExternalProxyEntry(host: HostLinkInput): ExternalProxyEntry {
  const forceTls = host.security === 'tls' || host.security === 'none' ? host.security : 'same';

  let sni: string | undefined;
  if (host.keepSniBlank) {
    sni = undefined;
  } else if (host.overrideSniFromAddress) {
    sni = host.address || undefined;
  } else {
    sni = host.sni || undefined;
  }

  return {
    forceTls,
    dest: host.address || '',
    port: host.port && host.port > 0 ? host.port : 443,
    remark: host.remark || '',
    sni,
    fingerprint: host.fingerprint,
    alpn: host.alpn && host.alpn.length > 0 ? host.alpn : undefined,
    pinnedPeerCertSha256:
      host.pinnedPeerCertSha256 && host.pinnedPeerCertSha256.length > 0
        ? host.pinnedPeerCertSha256
        : undefined,
    verifyPeerCertByName: host.verifyPeerCertByName || undefined,
    echConfigList: host.echConfigList || undefined,
    vlessRoute: host.vlessRoute || undefined,
    allowInsecure: host.allowInsecure || undefined,
  };
}

function splitAdvertisedHost(value: string, inboundPort: number): [string, number] {
  const host = value.trim();
  if (host.startsWith('[')) {
    const close = host.indexOf(']');
    if (close > 0) {
      const port = host.slice(close + 1).match(/^:(\d+)$/)?.[1];
      return [host.slice(1, close), port ? Number(port) : inboundPort];
    }
  }
  const match = host.match(/^([^:]*):(\d+)$/);
  return match ? [match[1], Number(match[2])] : [host, inboundPort];
}

export interface HostEndpoint {
  dest: string;
  port: number;
  remark: string;
  forceTls: ExternalProxyEntry['forceTls'];
  sni?: string;
  alpn?: string[];
  allowInsecure?: boolean;
  fingerprint?: string;
  pinnedPeerCertSha256?: string[];
  verifyPeerCertByName?: string;
  echConfigList?: string;
  vlessRoute?: string;
}

// hostEndpointsFor mirrors the backend hostEndpoints + hostToExternalProxyMap:
// enabled Hosts of that sub type only; a blank address or port inherits the inbound's.
export function hostEndpointsFor(
  records: HostRecord[],
  inboundId: number,
  inboundPort: number,
  defaultDest: string,
  subType: 'raw' | 'clash' = 'raw',
): HostEndpoint[] {
  const endpoints: HostEndpoint[] = [];
  for (const record of records) {
    if (
      record.isDisabled ||
      !record.inboundIds.includes(inboundId) ||
      record.excludeFromSubTypes?.includes(subType)
    ) {
      continue;
    }
    for (const value of record.hosts) {
      const [address, port] = splitAdvertisedHost(value, inboundPort);
      const dest = address || defaultDest;
      const endpoint: HostEndpoint = {
        dest,
        port,
        remark: record.remark || '',
        forceTls:
          record.security === 'tls' || record.security === 'none' ? record.security : 'same',
      };
      const sni = record.overrideSniFromAddress ? dest : record.sni;
      if (!record.keepSniBlank && sni) endpoint.sni = sni;
      if (record.alpn && record.alpn.length > 0) endpoint.alpn = record.alpn;
      if (record.allowInsecure) endpoint.allowInsecure = true;
      if (record.fingerprint) endpoint.fingerprint = record.fingerprint;
      if (record.pinnedPeerCertSha256 && record.pinnedPeerCertSha256.length > 0) {
        endpoint.pinnedPeerCertSha256 = record.pinnedPeerCertSha256;
      }
      if (record.verifyPeerCertByName) endpoint.verifyPeerCertByName = record.verifyPeerCertByName;
      if (record.echConfigList) endpoint.echConfigList = record.echConfigList;
      if (record.vlessRoute) endpoint.vlessRoute = record.vlessRoute;
      endpoints.push(endpoint);
    }
  }
  return endpoints;
}

export function withHostEndpoints(
  inbound: Inbound,
  inboundId: number,
  records: HostRecord[],
  hostOverride: string,
  fallbackHostname: string,
): Inbound {
  const defaultDest = resolveAddr(inbound, hostOverride, fallbackHostname);
  const endpoints = hostEndpointsFor(records, inboundId, inbound.port, defaultDest);
  if (endpoints.length === 0) return inbound;
  const externalProxy = endpoints.map(hostEndpointToEntry);
  return {
    ...inbound,
    streamSettings: { ...inbound.streamSettings, externalProxy },
  } as Inbound;
}

// The externalProxy entry the share-link generators read for one Host endpoint,
// with the overrides the subscription applies to it (hostToExternalProxyMap).
function hostEndpointToEntry(endpoint: HostEndpoint): ExternalProxyEntry {
  const fingerprint = UtlsFingerprintSchema.safeParse(endpoint.fingerprint);
  const alpn = (endpoint.alpn ?? []).flatMap((value) => {
    const parsed = AlpnSchema.safeParse(value);
    return parsed.success ? [parsed.data] : [];
  });
  return {
    forceTls: endpoint.forceTls,
    dest: endpoint.dest,
    port: endpoint.port,
    remark: endpoint.remark,
    isHost: true,
    ...(endpoint.sni ? { sni: endpoint.sni } : {}),
    ...(fingerprint.success ? { fingerprint: fingerprint.data } : {}),
    ...(alpn.length > 0 ? { alpn } : {}),
    ...(endpoint.pinnedPeerCertSha256
      ? { pinnedPeerCertSha256: endpoint.pinnedPeerCertSha256 }
      : {}),
    ...(endpoint.verifyPeerCertByName
      ? { verifyPeerCertByName: endpoint.verifyPeerCertByName }
      : {}),
    ...(endpoint.echConfigList ? { echConfigList: endpoint.echConfigList } : {}),
    ...(endpoint.vlessRoute ? { vlessRoute: endpoint.vlessRoute } : {}),
    ...(endpoint.allowInsecure ? { allowInsecure: true } : {}),
  };
}

/** The address and port people connect to: an inbound's enabled entries, else its own. */
export function publicEndpointsOf(
  inbound: {
    id: number;
    port: number;
    listen?: string;
    shareAddrStrategy?: string;
    shareAddr?: string;
  },
  records: HostRecord[],
  nodeAddress: string,
  fallbackHostname: string,
): string[] {
  const dest = resolveShareHost(inbound, nodeAddress, fallbackHostname);
  const endpoints = hostEndpointsFor(records, inbound.id, inbound.port, dest);
  const shown = endpoints.length > 0 ? endpoints : [{ dest, port: inbound.port }];
  return shown.map(({ dest: host, port }) =>
    host.includes(':') && !host.startsWith('[') ? `[${host}]:${port}` : `${host}:${port}`,
  );
}
