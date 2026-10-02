import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import HostPage from '@/pages/nodes/HostPage';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

// The top bar has its own tests and needs settings the page itself does not.
vi.mock('@/layouts/AppNav', () => ({ default: () => null }));

const HOSTS = [
  {
    id: 2,
    name: 'edge-hk',
    remark: 'HK NAT box',
    kind: 'agent',
    address: '203.0.113.53',
    enable: true,
    status: 'online',
  },
  {
    id: 3,
    name: 'edge-us',
    remark: 'US box',
    kind: 'agent',
    address: '203.0.113.17',
    enable: true,
    status: 'online',
  },
];

const inbound = (
  id: number,
  remark: string,
  port: number,
  nodeId: number | null,
  shareAddr: string,
) => ({
  id,
  remark,
  port,
  nodeId,
  protocol: 'vless',
  enable: true,
  listen: '',
  tag: `in-${id}`,
  shareAddrStrategy: 'custom',
  shareAddr,
  subSortIndex: id,
  up: 0,
  down: 0,
  total: 0,
  expiryTime: 0,
});

const INBOUNDS = [
  inbound(1, '洛杉矶-Core', 443, null, '198.51.100.19'),
  inbound(5, '香港-Edge', 81, 2, '203.0.113.53'),
  inbound(7, '美国-Edge', 10443, 3, '203.0.113.17'),
];

const ENTRIES = [
  { groupId: 'nat', inboundIds: [5], hosts: [':20443'], port: 20443, remark: '香港-Edge' },
];

// The setup file stubs HttpUtil for every test; only GET is answered here, and its
// stub is put back afterwards so the setup's POST stub keeps working.
const getStub = vi.mocked(HttpUtil.get);
const setupGet = getStub.getMockImplementation();

function serve() {
  getStub.mockImplementation(async (url: string) => {
    if (url === '/panel/api/inbounds/list/slim') return new Msg(true, '', INBOUNDS);
    if (url === '/panel/api/nodes/list') return new Msg(true, '', HOSTS);
    if (url === '/panel/api/hosts/list') return new Msg(true, '', ENTRIES);
    if (url === '/panel/api/inbounds/options') return new Msg(true, '', []);
    return new Msg(true, '', {});
  });
}

function renderAt(path: string) {
  serve();
  return renderWithProviders(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/nodes/:hostId" element={<HostPage />} />
        <Route path="/nodes" element={<div>hosts list</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  if (setupGet) getStub.mockImplementation(setupGet);
});

describe('HostPage', () => {
  it("lists only this host's nodes, with the endpoint people connect to", async () => {
    renderAt('/nodes/2');
    await screen.findByText('香港-Edge');
    expect(screen.getByRole('heading', { name: 'edge-hk' })).toBeTruthy();
    expect(screen.queryByText('美国-Edge')).toBeNull();
    expect(screen.queryByText('洛杉矶-Core')).toBeNull();
    expect(screen.getByText('203.0.113.53:20443')).toBeTruthy();
    // Import and export act on every inbound of the panel, not on this host's.
    expect(screen.queryByRole('button', { name: /General Actions$/ })).toBeNull();
  });

  it("lists the local panel's own nodes on its page", async () => {
    renderAt('/nodes/local');
    await screen.findByText('洛杉矶-Core');
    expect(screen.getByRole('heading', { name: 'Local panel' })).toBeTruthy();
    expect(screen.queryByText('香港-Edge')).toBeNull();
    expect(screen.getByText('198.51.100.19:443')).toBeTruthy();
  });

  // An empty list would read as "this host has no nodes" for a host that is gone.
  it('says so when the host does not exist', async () => {
    renderAt('/nodes/9');
    await screen.findByText('This host no longer exists');
    expect(screen.queryByText('香港-Edge')).toBeNull();
  });

  it('adds a node on this host from its page', async () => {
    renderAt('/nodes/2');
    await screen.findByText('香港-Edge');
    fireEvent.click(screen.getByRole('button', { name: /Add Inbound$/ }));
    const dialog = await screen.findByRole('dialog');
    await waitFor(() => expect(within(dialog).getByTitle('edge-hk')).toBeTruthy());
    expect(within(dialog).getByTitle('edge-hk').closest('.ant-select')?.className).toContain(
      'ant-select-disabled',
    );
  });
});
