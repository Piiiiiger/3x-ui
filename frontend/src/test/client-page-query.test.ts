import { expect, test } from 'vitest';

import { buildClientPageQuery, sameClientQuery, type ClientQueryParams } from '@/hooks/useClients';

// satisfies Required<> makes a new filter on ClientQueryParams fail to compile here
// until it is listed, and each test then loops over every filter.
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
  plan: '3',
} satisfies Required<ClientQueryParams>;

test('every filter the clients page can set reaches the list request', () => {
  const sent = new URLSearchParams(buildClientPageQuery(params));
  for (const [key, value] of Object.entries(params)) {
    expect(sent.get(key), key).toBe(String(value));
  }
});

// The list refetches only when the query changes, so a change to any one filter has to
// count: the plan filter alone used to leave the list as it was.
test('a change to any one filter makes a new list query', () => {
  expect(sameClientQuery(params, { ...params })).toBe(true);
  for (const [key, value] of Object.entries(params)) {
    const changed = { ...params, [key]: typeof value === 'number' ? value + 1 : `${value}x` };
    expect(sameClientQuery(params, changed as ClientQueryParams), key).toBe(false);
  }
});
