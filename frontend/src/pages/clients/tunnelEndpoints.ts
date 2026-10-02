import { preferPublicHost } from '@/lib/xray/inbound-link';
import { publicEndpointOf, endpointLabel, type PublicEndpoint } from '@/lib/xray/public-port';
import type { InboundOption } from '@/hooks/useClients';

// The client config a tunnel inbound gets: one dialing its public port behind NAT;
// `undefined` stands for the inbound's own address and port.
export function tunnelConfigEndpoints(
  inbound: InboundOption,
  host: string,
  publicHost: string,
): (PublicEndpoint | undefined)[] {
  return [publicEndpointOf(inbound, inbound.nodeAddress ?? '', preferPublicHost(host, publicHost))];
}

export function tunnelEndpointLabel(endpoint: PublicEndpoint | undefined): string {
  return endpoint ? endpointLabel(endpoint) : '';
}
