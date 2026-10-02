import { describe, expect, it } from 'vitest';

import type { ClientRecord, InboundOption } from '@/hooks/useClients';
import { genAmneziaWGPeerConfigs, genWireguardPeerConfigs } from '@/lib/xray/inbound-link';
import { withPublicPort } from '@/lib/xray/public-port';
import { buildAmneziaWGClientConfig } from '@/pages/clients/amneziawgConfig';
import { buildTuicClientConfig } from '@/pages/clients/tuicConfig';
import { tunnelConfigEndpoints } from '@/pages/clients/tunnelEndpoints';
import { buildWireguardClientConfig } from '@/pages/clients/wireguardConfig';
import { InboundSchema, type Inbound } from '@/schemas/api/inbound';

const PANEL = 'panel.example.com';
const NAT = { shareAddrStrategy: 'custom', shareAddr: 'nat.example.com', sharePort: 20443 };

function endpointLines(configs: string[]): string[] {
  return configs.map((cfg) => cfg.match(/^Endpoint = (.*)$/m)?.[1] ?? '');
}

// Behind NAT the panel's own tunnel previews dial the public port, as the
// subscription's configs do (withPublicPort in internal/sub).
describe('inbounds page tunnel configs advertise the public port', () => {
  const wireguard = (share: object) =>
    InboundSchema.parse({
      port: 51820,
      protocol: 'wireguard',
      ...share,
      settings: {
        secretKey: 'iJ2cBkrSGqRwIfYIDIxk7hr5RXfdR93MfJUL7yqkkH8=',
        peers: [],
        clients: [
          {
            email: 'alice',
            privateKey: 'QGVlb2dXc1ZTWGw0ZXBzZndsWmtMaUM5MUlNYjBHWFdYbz0=',
            allowedIPs: ['10.0.0.2/32'],
          },
        ],
      },
    });

  it('points the WireGuard Endpoint at the public port, and at its own port without one', () => {
    const peersFor = (share: object) =>
      genWireguardPeerConfigs({
        inbound: withPublicPort(wireguard(share), '', PANEL),
        remark: 'wg',
        fallbackHostname: PANEL,
      });
    expect(endpointLines(peersFor(NAT)[0])).toEqual(['nat.example.com:20443']);
    expect(endpointLines(peersFor({})[0])).toEqual([`${PANEL}:51820`]);
  });

  it('points the AmneziaWG Endpoint at the public port', () => {
    const inbound = {
      port: 51821,
      protocol: 'amneziawg',
      ...NAT,
      settings: {
        server: { publicKey: 'serverPubKey==', jc: 4, jmin: 40, jmax: 100, s1: 30, s2: 90 },
        clients: [{ email: 'alice', privateKey: 'clientPrivKey==', allowedIPs: ['10.8.1.2/32'] }],
      },
      streamSettings: {},
    } as unknown as Inbound;
    const peers = genAmneziaWGPeerConfigs({
      inbound: withPublicPort(inbound, '', PANEL),
      remark: 'awg',
      fallbackHostname: PANEL,
    });
    expect(endpointLines(peers[0])).toEqual(['nat.example.com:20443']);
  });
});

describe('clients page tunnel configs advertise the public port', () => {
  const client = {
    email: 'alice',
    privateKey: 'clientPrivKey==',
    allowedIPs: '10.0.0.2/32',
    uuid: 'e79b9107-1607-4e6c-a496-d8f99e4f0dc5',
    password: 'secret',
  } as unknown as ClientRecord;

  it('dials the public port in the WireGuard and AmneziaWG configs', () => {
    const wg = { id: 7, remark: 'wg', protocol: 'wireguard', port: 51820, ...NAT } as InboundOption;
    const awg = {
      id: 8,
      remark: 'awg',
      protocol: 'amneziawg',
      port: 51821,
      ...NAT,
      awgServer: { publicKey: 'serverPubKey==', jc: 4, jmin: 40, jmax: 100, s1: 30, s2: 90 },
    } as unknown as InboundOption;
    const wgConfigs = tunnelConfigEndpoints(wg, PANEL, '').map((ep) =>
      buildWireguardClientConfig(client, wg, PANEL, '', '', ep),
    );
    const awgConfigs = tunnelConfigEndpoints(awg, PANEL, '').map((ep) =>
      buildAmneziaWGClientConfig(client, awg, PANEL, '', '', ep),
    );
    expect(endpointLines(wgConfigs)).toEqual(['nat.example.com:20443']);
    expect(endpointLines(awgConfigs)).toEqual(['nat.example.com:20443']);
  });

  it('keeps the inbound address and port without a public port', () => {
    const inbound = { id: 9, remark: 'wg', protocol: 'wireguard', port: 51820 } as InboundOption;
    const configs = tunnelConfigEndpoints(inbound, PANEL, '').map((ep) =>
      buildWireguardClientConfig(client, inbound, PANEL, '', '', ep),
    );
    expect(endpointLines(configs)).toEqual([`${PANEL}:51820`]);
  });

  it('dials the public port in the TUIC config and keeps the inbound SNI and ALPN', () => {
    const inbound = {
      id: 7,
      remark: 'tuic',
      protocol: 'tuic',
      port: 8443,
      ...NAT,
      tuicServer: { sni: 'inbound.sni', alpn: ['h3'] },
    } as unknown as InboundOption;
    const [ep] = tunnelConfigEndpoints(inbound, PANEL, '');
    const cfg = buildTuicClientConfig(client, inbound, PANEL, '', ep);
    expect(cfg).toContain('server: nat.example.com');
    expect(cfg).toContain('port: 20443');
    expect(cfg).toContain('sni: inbound.sni');
    expect(cfg).toContain('alpn:\n      - h3\n');
  });
});
