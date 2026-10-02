import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import NodeList from '@/pages/nodes/NodeList';
import { LocalPanelCard, nodesByHostOf } from '@/pages/nodes/HostNodeChips';
import type { NodeRecord } from '@/schemas/node';

import { renderWithProviders } from './test-utils';

const noop = () => {};

function renderList(
  nodes: NodeRecord[],
  nodesByHost: Map<number, { id: number; remark?: string }[]>,
) {
  renderWithProviders(
    <MemoryRouter>
      <NodeList
        nodes={nodes}
        nodesByHost={nodesByHost}
        isMobile={false}
        selectedIds={[]}
        onSelectionChange={noop}
        showAddress={false}
        onShowAddressChange={noop}
        onEdit={noop}
        onDelete={noop}
        onProbe={noop}
        onToggleEnable={noop}
        onUpdateNode={noop}
      />
    </MemoryRouter>,
  );
}

describe('the hosts list', () => {
  // A host's nodes are managed on its own page, so the list must lead there.
  it('links each host to its own page and shows the nodes it runs', () => {
    renderList(
      [{ id: 2, name: 'edge-hk', kind: 'agent', enable: true, status: 'online', guid: 'g2' }],
      new Map([[2, [{ id: 5, remark: '香港-Edge' }]]]),
    );
    expect(screen.getByRole('link', { name: 'edge-hk' }).getAttribute('href')).toBe('/nodes/2');
    expect(screen.getByText('香港-Edge')).toBeTruthy();
  });

  // A sub-host is reached through another panel; this panel has no page for it.
  it('leaves a sub-host without a link', () => {
    renderList(
      [
        { id: 1, name: 'parent', enable: true, status: 'online', guid: 'p1' },
        { id: 0, name: 'child', guid: 'c1', parentGuid: 'p1', transitive: true },
      ],
      new Map(),
    );
    expect(screen.getByRole('link', { name: 'parent' })).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'child' })).toBeNull();
  });

  it('shows the first nodes by name and the rest as a count', () => {
    const many = Array.from({ length: 6 }, (_, i) => ({ id: 10 + i, remark: `node-${i}` }));
    renderList(
      [{ id: 2, name: 'edge-hk', enable: true, status: 'online', guid: 'g2' }],
      new Map([[2, many]]),
    );
    expect(screen.getByText('node-3')).toBeTruthy();
    expect(screen.queryByText('node-4')).toBeNull();
    expect(screen.getByText('+2')).toBeTruthy();
  });
});

describe('LocalPanelCard', () => {
  it("leads to the local panel's page with its nodes", () => {
    renderWithProviders(
      <MemoryRouter>
        <LocalPanelCard nodes={[{ id: 1, remark: '洛杉矶-Core' }]} />
      </MemoryRouter>,
    );
    expect(screen.getByRole('link', { name: 'Local panel' }).getAttribute('href')).toBe(
      '/nodes/local',
    );
    expect(screen.getByText('洛杉矶-Core')).toBeTruthy();
  });
});

describe('nodesByHostOf', () => {
  // An inbound without a host runs on this panel, so it belongs on the local card.
  it('groups nodes by host and puts the local panel under 0', () => {
    const byHost = nodesByHostOf([
      { id: 1, remark: 'Core', nodeId: null },
      { id: 5, remark: 'Edge', nodeId: 2 },
      { id: 6, remark: 'Edge-2', nodeId: 2 },
      { id: 8, remark: 'old row' },
    ]);
    expect(byHost.get(0)?.map((n) => n.id)).toEqual([1, 8]);
    expect(byHost.get(2)?.map((n) => n.id)).toEqual([5, 6]);
  });
});
