/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';

import { hostToExternalProxyEntry, withHostEndpoints } from '@/lib/hosts/host-link';
import { inboundFromDb } from '@/lib/xray/inbound-from-db';
import { genAllLinks, getInboundClients } from '@/lib/xray/inbound-link';
import type { HostRecord } from '@/schemas/api/host';

describe('hostToExternalProxyEntry', () => {
  const base = {
    security: 'tls' as const,
    address: 'cdn.example.com',
    port: 8443,
    remark: 'R',
    sni: 'sni.example.com',
    alpn: ['h2'] as ('h2' | 'h3' | 'http/1.1')[],
    fingerprint: 'chrome' as const,
    pinnedPeerCertSha256: ['AAAA'],
    verifyPeerCertByName: 'verify.example.com',
    echConfigList: 'ECH',
    overrideSniFromAddress: false,
    keepSniBlank: false,
    vlessRoute: '',
    allowInsecure: false,
  };

  it('maps the overlapping fields onto an external-proxy entry', () => {
    const ep = hostToExternalProxyEntry(base);
    expect(ep.forceTls).toBe('tls');
    expect(ep.dest).toBe('cdn.example.com');
    expect(ep.port).toBe(8443);
    expect(ep.remark).toBe('R');
    expect(ep.sni).toBe('sni.example.com');
    expect(ep.alpn).toEqual(['h2']);
    expect(ep.fingerprint).toBe('chrome');
    expect(ep.pinnedPeerCertSha256).toEqual(['AAAA']);
    expect(ep.verifyPeerCertByName).toBe('verify.example.com');
    expect(ep.echConfigList).toBe('ECH');
  });

  it('maps reality/same security to forceTls "same"', () => {
    expect(hostToExternalProxyEntry({ ...base, security: 'reality' }).forceTls).toBe('same');
    expect(hostToExternalProxyEntry({ ...base, security: 'same' }).forceTls).toBe('same');
    expect(hostToExternalProxyEntry({ ...base, security: 'none' }).forceTls).toBe('none');
  });

  it('uses the address as sni when overrideSniFromAddress is set', () => {
    const ep = hostToExternalProxyEntry({ ...base, overrideSniFromAddress: true });
    expect(ep.sni).toBe('cdn.example.com');
  });

  it('omits sni when keepSniBlank is set', () => {
    const ep = hostToExternalProxyEntry({ ...base, keepSniBlank: true });
    expect(ep.sni).toBeUndefined();
  });

  it('falls back to port 443 when the host port is 0 (inherit)', () => {
    const ep = hostToExternalProxyEntry({ ...base, port: 0 });
    expect(ep.port).toBe(443);
  });

  it('carries a single vlessRoute value through to the entry', () => {
    expect(hostToExternalProxyEntry({ ...base, vlessRoute: '443' }).vlessRoute).toBe('443');
    expect(hostToExternalProxyEntry({ ...base, vlessRoute: '' }).vlessRoute).toBeUndefined();
  });

  it('carries allowInsecure through to the entry', () => {
    expect(hostToExternalProxyEntry({ ...base, allowInsecure: true }).allowInsecure).toBe(true);
    expect(
      hostToExternalProxyEntry({ ...base, allowInsecure: false }).allowInsecure,
    ).toBeUndefined();
  });
});

describe('withHostEndpoints', () => {
  const inbound = inboundFromDb({
    protocol: 'mtproto',
    port: 4060,
    listen: '127.0.0.1',
    settings: { clients: [] },
    streamSettings: {},
    sniffing: {},
  });

  it('projects enabled raw Hosts onto MTProto share endpoints', () => {
    const got = withHostEndpoints(
      inbound,
      7,
      [
        {
          groupId: 'public',
          inboundIds: [7],
          hosts: ['proxy.example.com:443', '[2001:db8::1]'],
          port: 443,
          remark: 'public',
        },
      ],
      '',
      'panel.example.com',
    );
    expect(got.streamSettings?.externalProxy).toEqual([
      { forceTls: 'same', dest: 'proxy.example.com', port: 443, remark: 'public', isHost: true },
      { forceTls: 'same', dest: '2001:db8::1', port: 4060, remark: 'public', isHost: true },
    ]);
  });

  it('inherits the inbound address for a port-only Host', () => {
    const got = withHostEndpoints(
      inbound,
      7,
      [{ groupId: 'port-only', inboundIds: [7], hosts: [':8443'], port: 8443 }],
      '',
      'panel.example.com',
    );
    expect(got.streamSettings?.externalProxy).toEqual([
      { forceTls: 'same', dest: 'panel.example.com', port: 8443, remark: '', isHost: true },
    ]);
  });

  it('ignores disabled, excluded and unrelated Hosts', () => {
    const got = withHostEndpoints(
      inbound,
      7,
      [
        { groupId: 'disabled', inboundIds: [7], hosts: ['a.example.com:443'], isDisabled: true },
        {
          groupId: 'excluded',
          inboundIds: [7],
          hosts: ['b.example.com:443'],
          excludeFromSubTypes: ['raw'],
        },
        { groupId: 'other', inboundIds: [8], hosts: ['c.example.com:443'] },
      ],
      '',
      'panel.example.com',
    );
    expect(got).toBe(inbound);
  });
});

describe('withHostEndpoints on a VLESS REALITY inbound', () => {
  // The provider's NAT forwards 20443 to the inbound's port 81.
  const natNode = inboundFromDb({
    protocol: 'vless',
    port: 81,
    listen: '',
    settings: {
      clients: [
        { id: '11111111-1111-1111-1111-111111111111', email: 'alice', flow: 'xtls-rprx-vision' },
      ],
      decryption: 'none',
    },
    streamSettings: {
      network: 'tcp',
      security: 'reality',
      realitySettings: {
        target: 'www.bing.com:443',
        serverNames: ['www.bing.com'],
        privateKey: 'priv',
        shortIds: ['ab12'],
        settings: { publicKey: 'pub', fingerprint: 'chrome', spiderX: '/' },
      },
    },
    sniffing: {},
  });

  const linksFor = (records: HostRecord[]) => {
    const inbound = withHostEndpoints(natNode, 5, records, '203.0.113.53', 'panel.example.com');
    const [client] = getInboundClients(inbound) ?? [];
    return genAllLinks({
      inbound,
      remark: 'HK',
      client,
      hostOverride: '203.0.113.53',
      fallbackHostname: 'panel.example.com',
    }).map((entry) => new URL(entry.link));
  };

  const natEntry = (extra: Partial<HostRecord> = {}): HostRecord => ({
    groupId: 'nat',
    inboundIds: [5],
    hosts: [':20443'],
    port: 20443,
    remark: 'HK',
    security: 'same',
    ...extra,
  });

  // The panel's own QR showed :81 here while subscriptions gave :20443, so the
  // code people scanned from the panel pointed at a port nobody forwards.
  it("puts an entry's public port into the link and keeps REALITY", () => {
    const [link] = linksFor([natEntry()]);
    expect(link.host).toBe('203.0.113.53:20443');
    expect(link.searchParams.get('security')).toBe('reality');
    expect(link.searchParams.get('pbk')).toBe('pub');
    expect(link.searchParams.get('sid')).toBe('ab12');
    expect(link.searchParams.get('sni')).toBe('www.bing.com');
  });

  it('drops the REALITY parameters for an entry that forces no TLS', () => {
    const [link] = linksFor([natEntry({ security: 'none' })]);
    expect(link.searchParams.get('security')).toBe('none');
    expect(link.searchParams.get('pbk')).toBeNull();
    expect(link.searchParams.get('sid')).toBeNull();
  });

  it("lets an entry's SNI and fingerprint replace the REALITY ones, as subscriptions do", () => {
    const [link] = linksFor([natEntry({ sni: 'www.microsoft.com', fingerprint: 'firefox' })]);
    expect(link.searchParams.get('sni')).toBe('www.microsoft.com');
    expect(link.searchParams.get('fp')).toBe('firefox');
  });

  it('asks the client to skip certificate checks only for an entry that opts in', () => {
    expect(linksFor([natEntry({ allowInsecure: true })])[0].searchParams.get('allowInsecure')).toBe(
      '1',
    );
    expect(linksFor([natEntry()])[0].searchParams.get('allowInsecure')).toBeNull();
    expect(
      linksFor([natEntry({ allowInsecure: true, security: 'none' })])[0].searchParams.get(
        'allowInsecure',
      ),
    ).toBeNull();
  });

  // Subscriptions apply a legacy externalProxy entry's SNI to TLS links only, so a
  // REALITY link keeps the SNI of the target it imitates.
  it("keeps REALITY's SNI for a legacy externalProxy entry", () => {
    const legacy = inboundFromDb({
      ...natNode,
      streamSettings: {
        ...natNode.streamSettings,
        externalProxy: [
          {
            forceTls: 'same',
            dest: 'cdn.example.com',
            port: 443,
            remark: '',
            sni: 'legacy.example.com',
          },
        ],
      },
    });
    const [client] = getInboundClients(legacy) ?? [];
    const [entry] = genAllLinks({ inbound: legacy, client, fallbackHostname: 'panel.example.com' });
    expect(new URL(entry.link).searchParams.get('sni')).toBe('www.bing.com');
  });

  it('keeps the inbound address and port when every entry is disabled', () => {
    const [link] = linksFor([natEntry({ isDisabled: true })]);
    expect(link.host).toBe('203.0.113.53:81');
  });
});
