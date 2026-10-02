import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import GenerateNodeModal from '@/pages/nodes/GenerateNodeModal';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

const OPTIONS = [
  { id: 1, remark: '洛杉矶-Core', protocol: 'vless', security: 'reality', nodeId: null, port: 443 },
  {
    id: 5,
    remark: '香港-Edge',
    protocol: 'vless',
    security: 'reality',
    nodeId: 2,
    port: 81,
    sharePort: 20443,
  },
  {
    id: 7,
    remark: '美国-Edge',
    protocol: 'vless',
    security: 'reality',
    nodeId: 3,
    port: 10443,
  },
];
const plan = (id: number, name: string, inboundIds: number[], memberCount: number) => ({
  id,
  name,
  inboundIds,
  memberCount,
  clashRules: '',
  createdAt: 0,
  durationDays: 30,
  limitIp: 0,
  remark: '',
  sortIndex: 0,
  totalGB: 0,
  trafficReset: 'never',
  trafficResetDay: 0,
  updatedAt: 0,
});
const PLANS = [plan(11, 'Asia', [1, 5], 3), plan(12, 'US', [7], 2)];
const TEMPLATE = {
  id: 5,
  remark: '香港-Edge',
  port: 81,
  protocol: 'vless',
  listen: '',
  nodeId: 2,
  enable: true,
  tag: 'n2-in-81-tcp',
  shareAddrStrategy: 'custom',
  shareAddr: '203.0.113.53',
  up: 0,
  down: 0,
  total: 0,
  expiryTime: 0,
  settings: JSON.stringify({
    clients: [{ id: 'u-1', email: 'alice', flow: 'xtls-rprx-vision' }],
    decryption: 'none',
  }),
  streamSettings: JSON.stringify({
    network: 'tcp',
    security: 'reality',
    realitySettings: {
      target: 'www.bing.com:443',
      serverNames: ['www.bing.com'],
      privateKey: 'old-priv',
      shortIds: ['aa'],
      mldsa65Seed: '',
      settings: { publicKey: 'old-pub', fingerprint: 'chrome', spiderX: '/' },
    },
  }),
  sniffing: JSON.stringify({ enabled: false }),
};

const getStub = vi.mocked(HttpUtil.get);
const postStub = vi.mocked(HttpUtil.post);
const setupGet = getStub.getMockImplementation();
const setupPost = postStub.getMockImplementation();

beforeEach(() => {
  getStub.mockImplementation(async (url: string) => {
    if (url === '/panel/api/inbounds/options') return new Msg(true, '', OPTIONS);
    if (url === '/panel/api/plans/list') return new Msg(true, '', PLANS);
    if (url === '/panel/api/inbounds/freePort/2') return new Msg(true, '', { port: 24567 });
    if (url === '/panel/api/inbounds/get/5') return new Msg(true, '', TEMPLATE);
    if (url === '/panel/api/server/getNewX25519Cert') {
      return new Msg(true, '', { privateKey: 'new-priv', publicKey: 'new-pub' });
    }
    return new Msg(false, `unexpected GET ${url}`);
  });
});

afterEach(() => {
  if (setupGet) getStub.mockImplementation(setupGet);
  if (setupPost) postStub.mockImplementation(setupPost);
  postStub.mockClear();
});

function field(label: string): HTMLInputElement {
  const item = screen.getByText(label, { selector: 'label, label *' }).closest('.ant-form-item');
  if (!item) throw new Error(`no form item labelled ${label}`);
  return item.querySelector('input') as HTMLInputElement;
}

async function openFor(onClose = vi.fn()) {
  renderWithProviders(
    <GenerateNodeModal open host={{ id: 2, name: 'edge-hk' }} onClose={onClose} />,
  );
  await waitFor(() => expect(field('Port').value).toBe('24567'));
  return onClose;
}

const generateCalls = () =>
  postStub.mock.calls.filter(([url]) => url === '/panel/api/inbounds/generate');

describe('GenerateNodeModal', () => {
  it('fills in the next node of this host the way its first one is built', async () => {
    await openFor();
    expect(field('Name').value).toBe('香港-Edge-2');
    expect(field('REALITY target').value).toBe('www.bing.com:443');
    expect(field('SNI').value).toBe('www.bing.com');
    expect(screen.getByRole('checkbox', { name: 'Asia' })).toHaveProperty('checked', true);
    expect(screen.getByRole('checkbox', { name: 'US' })).toHaveProperty('checked', false);
    expect(screen.getByText('3 client(s) will get this node')).toBeTruthy();
  });

  // A NAT provider forwards one public port to one inside port; a node with
  // no forwarded public port would be listed in subscriptions and never reachable.
  it('needs the public port on a host behind NAT, then generates the node', async () => {
    const onClose = await openFor();
    postStub.mockResolvedValue(new Msg(true, '', { id: 9 }));

    fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
    await screen.findByText('Enter the public port the provider forwards to this node');
    expect(generateCalls()).toHaveLength(0);

    fireEvent.change(field('Public port'), { target: { value: '29236' } });
    fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
    await waitFor(() => expect(generateCalls()).toHaveLength(1));

    const [, body, options] = generateCalls()[0] as [
      string,
      Record<string, unknown>,
      { headers: Record<string, string> },
    ];
    expect(options.headers['Content-Type']).toBe('application/json');
    expect(body.planIds).toEqual([11]);
    const inbound = body.inbound as Record<string, unknown>;
    expect(inbound).toMatchObject({
      remark: '香港-Edge-2',
      port: 24567,
      sharePort: 29236,
      nodeId: 2,
    });
    expect(JSON.parse(inbound.settings as string).clients).toEqual([]);
    const reality = JSON.parse(inbound.streamSettings as string).realitySettings;
    expect(reality).toMatchObject({
      target: 'www.bing.com:443',
      serverNames: ['www.bing.com'],
      privateKey: 'new-priv',
    });
    expect(reality.shortIds).not.toEqual(['aa']);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("keeps the dialog open with the host's answer when the node was refused", async () => {
    const onClose = await openFor();
    postStub.mockResolvedValue(
      new Msg(
        false,
        'the node was left disabled: agent on edge-hk refused its config: bind: address already in use',
      ),
    );
    fireEvent.change(field('Public port'), { target: { value: '29236' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
    });
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText(/agent on edge-hk refused its config/);
    expect(onClose).not.toHaveBeenCalled();
  });
});
