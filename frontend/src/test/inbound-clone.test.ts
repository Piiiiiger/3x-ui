import { describe, expect, it } from 'vitest';

import {
  buildClonePayload,
  cloneShareFor,
  hostShares,
  pickClonePort,
} from '@/lib/xray/inbound-clone';
import { DBInbound } from '@/models/dbinbound';

function sourceInbound() {
  return new DBInbound({
    id: 7,
    port: 443,
    listen: '0.0.0.0',
    protocol: 'vless',
    remark: 'edge',
    enable: true,
    settings: JSON.stringify({
      clients: [{ id: 'uuid-1', email: 'a@test', flow: 'xtls-rprx-vision' }],
      decryption: 'none',
    }),
    streamSettings: {
      network: 'tcp',
      security: 'reality',
      realitySettings: { dest: 'www.lovelive-anime.jp:443' },
    },
    sniffing: { enabled: true },
    nodeId: 2,
    shareAddrStrategy: 'node',
    shareAddr: '',
  });
}

const NODE_ADDRESS = { shareAddrStrategy: 'node', shareAddr: '' };

describe('buildClonePayload', () => {
  it('omits nodeId for a local-panel target so the row stays panel-local', () => {
    const payload = buildClonePayload(sourceInbound(), 23456, null, NODE_ADDRESS);
    expect(payload).not.toHaveProperty('nodeId');
  });

  it('carries nodeId for a node target', () => {
    const payload = buildClonePayload(sourceInbound(), 23456, 5, NODE_ADDRESS);
    expect(payload.nodeId).toBe(5);
  });

  it('stages the clone disabled with cleared clients, fresh port, and no tag', () => {
    const payload = buildClonePayload(sourceInbound(), 23456, 3, NODE_ADDRESS);
    expect(payload.enable).toBe(false);
    expect(payload.port).toBe(23456);
    expect(payload.listen).toBe('');
    expect(payload).not.toHaveProperty('tag');
    expect(payload.remark).toBe('edge (clone)');

    const settings = JSON.parse(payload.settings);
    // Clients are dropped (emails are unique panel-wide, UUIDs must not
    // repeat across nodes) while the rest of the settings survive verbatim.
    expect(settings.clients).toEqual([]);
    expect(settings.decryption).toBe('none');
  });

  it('stringifies object-shaped streamSettings and sniffing from hydrated rows', () => {
    const payload = buildClonePayload(sourceInbound(), 23456, null, NODE_ADDRESS);
    expect(JSON.parse(payload.streamSettings)).toEqual({
      network: 'tcp',
      security: 'reality',
      realitySettings: { dest: 'www.lovelive-anime.jp:443' },
    });
    expect(JSON.parse(payload.sniffing)).toEqual({ enabled: true });
  });

  it('survives malformed settings JSON with an empty client list fallback', () => {
    const broken = sourceInbound();
    broken.settings = '{not json';
    const payload = buildClonePayload(broken, 23456, null, NODE_ADDRESS);
    const settings = JSON.parse(payload.settings);
    expect(settings.clients ?? []).toEqual([]);
  });
});

describe('pickClonePort', () => {
  it('never returns a port already bound on the target', () => {
    const used = new Set<number>();
    for (let p = 10000; p <= 60000; p++) if (p !== 23456) used.add(p);
    expect(pickClonePort(used)).toBe(23456);
  });

  it('stops probing when the range looks exhausted instead of spinning', () => {
    const used = new Set<number>();
    for (let p = 10000; p <= 60000; p++) used.add(p);
    const port = pickClonePort(used);
    expect(port).toBeGreaterThanOrEqual(10000);
    expect(port).toBeLessThanOrEqual(60000);
  });
});

function realitySource() {
  const src = sourceInbound();
  src.streamSettings = JSON.stringify({
    network: 'tcp',
    security: 'reality',
    realitySettings: {
      target: 'www.bing.com:443',
      serverNames: ['www.bing.com'],
      privateKey: 'src-priv',
      shortIds: ['aa'],
      mldsa65Seed: 'src-seed',
      settings: {
        publicKey: 'src-pub',
        spiderX: '/src',
        mldsa65Verify: 'src-verify',
        fingerprint: 'chrome',
      },
    },
  });
  return src;
}

describe('buildClonePayload with fresh REALITY values', () => {
  // A copy that kept the source's private key and short ids would let anyone
  // holding one node's link pass REALITY on the other as well.
  it('replaces every REALITY secret and keeps the target the copy imitates', () => {
    const payload = buildClonePayload(realitySource(), 23456, 5, NODE_ADDRESS, {
      privateKey: 'new-priv',
      publicKey: 'new-pub',
      shortIds: ['bb'],
      spiderX: '/new',
      mldsa65: { seed: 'new-seed', verify: 'new-verify' },
    });
    const reality = JSON.parse(payload.streamSettings).realitySettings;
    expect(reality).toMatchObject({
      target: 'www.bing.com:443',
      serverNames: ['www.bing.com'],
      privateKey: 'new-priv',
      shortIds: ['bb'],
      mldsa65Seed: 'new-seed',
    });
    expect(reality.settings).toEqual({
      publicKey: 'new-pub',
      spiderX: '/new',
      mldsa65Verify: 'new-verify',
      fingerprint: 'chrome',
    });
  });

  it('drops an ML-DSA seed it was given no fresh one for instead of sharing it', () => {
    const payload = buildClonePayload(realitySource(), 23456, 5, NODE_ADDRESS, {
      privateKey: 'new-priv',
      publicKey: 'new-pub',
      shortIds: ['bb'],
      spiderX: '/new',
    });
    const reality = JSON.parse(payload.streamSettings).realitySettings;
    expect(reality.mldsa65Seed).toBe('');
    expect(reality.settings.mldsa65Verify).toBe('');
  });
});

describe('cloneShareFor', () => {
  const shares = new Map([[0, { shareAddrStrategy: 'custom', shareAddr: '198.51.100.19' }]]);
  const relaySource = () => {
    const src = sourceInbound();
    src.shareAddrStrategy = 'custom';
    src.shareAddr = '198.51.100.97';
    return src;
  };

  it('keeps the source address for a copy on the same host', () => {
    expect(cloneShareFor(relaySource(), 2, shares)).toEqual({
      shareAddrStrategy: 'custom',
      shareAddr: '198.51.100.97',
    });
  });

  // Copying the source's custom address to another host advertised the source
  // host's IP in every link of the copy.
  it('takes the address a node already on the target host advertises', () => {
    expect(cloneShareFor(relaySource(), 0, shares)).toEqual({
      shareAddrStrategy: 'custom',
      shareAddr: '198.51.100.19',
    });
  });

  it("falls back to the target host's own address when it has no node yet", () => {
    expect(cloneShareFor(relaySource(), 9, shares)).toEqual({
      shareAddrStrategy: 'node',
      shareAddr: '',
    });
  });
});

describe('hostShares', () => {
  it("takes each host's first node in subscription order", () => {
    const node = (id: number, nodeId: number | null, subSortIndex: number, shareAddr: string) =>
      new DBInbound({ id, nodeId, subSortIndex, shareAddrStrategy: 'custom', shareAddr });
    const shares = hostShares([
      node(1, null, 5, 'later'),
      node(3, null, 2, 'first'),
      node(4, 2, 1, 'relay-hk'),
    ]);
    expect(shares.get(0)?.shareAddr).toBe('first');
    expect(shares.get(2)?.shareAddr).toBe('relay-hk');
  });
});
