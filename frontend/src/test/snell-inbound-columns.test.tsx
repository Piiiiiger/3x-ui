import type { ReactNode } from 'react';
import { screen } from '@testing-library/react';
import { expect, it } from 'vitest';

import type { DBInboundRecord } from '@/pages/inbounds/list/types';
import { useInboundColumns } from '@/pages/inbounds/list/useInboundColumns';
import { renderWithProviders } from './test-utils';

const MB = 1024 * 1024;

function snellRow(over: Partial<DBInboundRecord> = {}): DBInboundRecord {
  return {
    id: 11,
    enable: true,
    remark: 'HK Snell',
    subSortIndex: 0,
    port: 26163,
    protocol: 'snell',
    up: 100 * MB,
    down: 900 * MB,
    total: 0,
    expiryTime: 0,
    _expiryTime: null,
    nodeId: 3,
    settings: {},
    streamSettings: {},
    ...over,
  };
}

function Cell({ column, record }: { column: string; record: DBInboundRecord }) {
  const columns = useInboundColumns({
    hasAnyRemark: false,
    hasAnySubSortIndex: false,
    hasActiveNode: true,
    nodesById: new Map(),
    clientCount: {},
    inboundSpeed: {},
    subEnable: false,
    expireDiff: 0,
    trafficDiff: 0,
    onRowAction: () => {},
    onSwitchEnable: () => {},
  });
  const col = columns.find((c) => c.key === column);
  return <>{col?.render?.(undefined, record, 0) as ReactNode}</>;
}

it("shows a Snell inbound's counted traffic like any other inbound's", () => {
  renderWithProviders(<Cell column="traffic" record={snellRow()} />);
  expect(screen.getByText(/1000\.00 MB/)).toBeTruthy();
});

it('says a Snell inbound is suspended for its only user', () => {
  renderWithProviders(<Cell column="protocol" record={snellRow({ runtimeState: 'suspended' })} />);
  expect(screen.getByText('Suspended')).toBeTruthy();
});
