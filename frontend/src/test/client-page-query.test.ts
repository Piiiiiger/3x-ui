import { expect, test } from 'vitest';

import { buildClientPageQuery, type ClientQueryParams } from '@/hooks/useClients';

// satisfies Required<> makes a new filter on ClientQueryParams fail to compile here
// until it is listed, and the loop then proves the request actually carries it.
test('every filter the clients page can set reaches the list request', () => {
  const params = {
    page: 2,
    pageSize: 50,
    search: 'alice',
    filter: 'active',
    protocol: 'vless',
    inbound: '1',
    sort: 'email',
    order: 'ascend',
    expiryFrom: 1,
    expiryTo: 2,
    usageFrom: 3,
    usageTo: 4,
    autoRenew: 'on',
    hasTgId: 'yes',
    hasComment: 'no',
    group: 'vip',
    plan: '3',
  } satisfies Required<ClientQueryParams>;

  const sent = new URLSearchParams(buildClientPageQuery(params));
  for (const [key, value] of Object.entries(params)) {
    expect(sent.get(key), key).toBe(String(value));
  }
});
