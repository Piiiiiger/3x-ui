import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { parse } from 'yaml';
import PortalCustomize, { editRouteYaml, readRouteYaml } from '@/pages/sub/portal/PortalCustomize';
import { renderWithProviders } from './test-utils';

vi.mock('@/components/form', () => ({
  YamlEditor: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <textarea aria-label="YAML" value={value} onChange={(e) => onChange(e.target.value)} />
  ),
}));
const yaml =
  'proxy-groups:\n  - name: Main\n    type: select\n    proxies: [DIRECT]\nrules:\n  - MATCH,Main\nrule-providers:\n  demo:\n    type: http\n    url: https://example.com/rules.yaml\n';
const snapshot = {
  nodesYaml: '',
  links: [{ kind: 'link', value: 'vless://existing', remark: 'Existing' }],
  rulesYaml: '',
  effectiveYaml: yaml,
  groups: [{ name: 'Main', type: 'select', proxies: ['DIRECT'] }],
  rules: ['MATCH,Main'],
  nodeCount: 1,
  nodes: [],
  proxyNames: ['Existing'],
  updatedAt: 1,
  versions: [],
};
afterEach(() => vi.unstubAllGlobals());
it('preserves providers and advanced group options during visual edits', () => {
  const route = readRouteYaml(yaml);
  route.groups[0].extra = { interval: 120, 'include-all-proxies': true };
  const next = parse(
    editRouteYaml(yaml, route.groups, ['DOMAIN-SUFFIX,example.org,DIRECT', ...route.rules]),
  );
  expect(next['rule-providers'].demo.url).toBe('https://example.com/rules.yaml');
  expect(next['proxy-groups'][0]['include-all-proxies']).toBe(true);
  expect(next.rules[0]).toBe('DOMAIN-SUFFIX,example.org,DIRECT');
  expect(() => readRouteYaml('rules: [broken')).toThrow();
});
it('loads saved links, hides advanced YAML and saves only explicitly added sources', async () => {
  const fetcher = vi.fn(
    async (_url, init) =>
      new Response(
        JSON.stringify(
          init?.method === 'PUT'
            ? { ...snapshot, ...JSON.parse(init.body), updatedAt: 2 }
            : snapshot,
        ),
        { status: 200 },
      ),
  );
  vi.stubGlobal('fetch', fetcher);
  renderWithProviders(<PortalCustomize base="/x/portal" onSessionEnded={() => {}} />);
  await screen.findByDisplayValue('Existing');
  expect(screen.queryByLabelText('YAML')).toBeNull();
  expect(
    (screen.getAllByText('保存并应用')[0].closest('button') as HTMLButtonElement).disabled,
  ).toBe(true);
  fireEvent.change(screen.getByLabelText('节点链接或订阅地址'), {
    target: { value: 'vless://new-node' },
  });
  fireEvent.click(screen.getByRole('button', { name: /添加$/ }));
  await screen.findByText('有未保存的修改');
  fireEvent.click(screen.getAllByRole('button', { name: /保存并应用$/ })[0]);
  await waitFor(() =>
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true),
  );
  const payload = JSON.parse(
    fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')![1].body,
  );
  expect(payload.links).toHaveLength(2);
  expect(payload.rulesYaml).toBe('');
  await waitFor(() =>
    expect(
      (screen.getAllByText('保存并应用')[0].closest('button') as HTMLButtonElement).disabled,
    ).toBe(true),
  );
});
