import { describe, expect, it } from 'vitest';

import {
  isNatHost,
  pickTemplate,
  pretickedPlans,
  uniqueNodeName,
  withRealityTarget,
} from '@/pages/nodes/generateNode';
import type { PlanSummary } from '@/generated/zod';
import type { HostRecord } from '@/schemas/api/host';
import type { InboundOption } from '@/schemas/client';

describe('uniqueNodeName', () => {
  // The name is the Clash proxy name: two nodes sharing one collide in clients.
  it('numbers a name that is already taken', () => {
    expect(uniqueNodeName('香港-Relay', ['洛杉矶-Core'])).toBe('香港-Relay');
    expect(uniqueNodeName('香港-Relay', ['香港-Relay'])).toBe('香港-Relay-2');
    expect(uniqueNodeName('香港-Relay', ['香港-Relay', '香港-Relay-2'])).toBe('香港-Relay-3');
  });
});

describe('pickTemplate', () => {
  const option = (
    id: number,
    nodeId: number | null,
    security: string,
    protocol = 'vless',
  ): InboundOption => ({
    id,
    nodeId,
    security,
    protocol,
  });

  it('copies a REALITY node of the same host, else one from another host', () => {
    const options = [option(1, null, 'reality'), option(4, 3, 'reality'), option(5, 2, 'reality')];
    expect(pickTemplate(options, 2)?.id).toBe(5);
    expect(pickTemplate(options, 9)?.id).toBe(1);
  });

  it('never copies a node that is not VLESS over REALITY', () => {
    const options = [option(1, 2, 'tls'), option(2, 2, 'reality', 'trojan')];
    expect(pickTemplate(options, 2)).toBeUndefined();
  });
});

describe('pretickedPlans', () => {
  const plan = (id: number, inboundIds: number[]) => ({ id, inboundIds }) as PlanSummary;

  // A plan that has every node of a host gives its people that host; one that
  // lacks a node there was given only part of it on purpose.
  it('ticks only plans that already have every node of the host', () => {
    expect(pretickedPlans([plan(1, [5, 6, 9]), plan(2, [5]), plan(3, [9])], [5, 6])).toEqual([1]);
  });

  it('ticks nothing for a host without nodes', () => {
    expect(pretickedPlans([plan(1, [5])], [])).toEqual([]);
  });
});

describe('isNatHost', () => {
  const natNode = [{ id: 5, port: 81 }];
  const entry = (extra: Partial<HostRecord>): HostRecord => ({
    groupId: 'g',
    inboundIds: [5],
    hosts: [':20443'],
    ...extra,
  });

  it('spots an entry that moves a node of the host to another public port', () => {
    expect(isNatHost(natNode, [entry({ port: 20443 })])).toBe(true);
  });

  it('ignores entries that keep the port, are disabled, or belong elsewhere', () => {
    expect(isNatHost(natNode, [])).toBe(false);
    expect(isNatHost(natNode, [entry({ hosts: ['cdn.example.com'], port: 81 })])).toBe(false);
    expect(isNatHost(natNode, [entry({ port: 20443, isDisabled: true })])).toBe(false);
    expect(isNatHost(natNode, [entry({ port: 20443, inboundIds: [7] })])).toBe(false);
  });
});

describe('withRealityTarget', () => {
  it('sets the target and server name and keeps the rest of REALITY', () => {
    const stream = JSON.stringify({
      network: 'tcp',
      security: 'reality',
      realitySettings: {
        target: 'www.cisco.com:443',
        serverNames: ['www.cisco.com'],
        privateKey: 'k',
      },
    });
    const reality = JSON.parse(
      withRealityTarget(stream, 'www.bing.com:443', 'www.bing.com'),
    ).realitySettings;
    expect(reality).toEqual({
      target: 'www.bing.com:443',
      serverNames: ['www.bing.com'],
      privateKey: 'k',
    });
  });
});
