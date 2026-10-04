import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { setupHttp } from '@/api/http-init';
import ChainsPage from '@/pages/chains/ChainsPage';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

vi.mock('@/layouts/AppNav', () => ({ default: () => null }));
vi.mock('antd', async (importOriginal) => {
  const antd = await importOriginal<typeof import('antd')>();
  return {
    ...antd,
    Select: ({
      id,
      value,
      onChange,
      options,
    }: {
      id?: string;
      value?: number;
      onChange?: (value: number) => void;
      options: { value: number; label: string }[];
    }) => (
      <select
        id={id}
        value={value ?? ''}
        onChange={(event) => onChange?.(Number(event.target.value))}
      >
        <option value="">请选择</option>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    ),
  };
});

// Exercise the real request serializer; a mocked post would miss this regression.
vi.mocked(HttpUtil.post).mockRestore();
const getStub = vi.mocked(HttpUtil.get);
const originalGet = getStub.getMockImplementation();
const fetchMock = vi.fn<typeof fetch>();
const chain = {
  id: 7,
  name: '新加坡奶爸中转',
  targetInboundId: 6,
  relayInboundId: 4,
  enabled: true,
};

beforeEach(() => {
  document.head.innerHTML = '<meta name="csrf-token" content="test-token">';
  setupHttp();
  fetchMock.mockReset();
  fetchMock.mockImplementation(
    async () =>
      new Response(JSON.stringify({ success: true, msg: '', obj: chain }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  );
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
  if (originalGet) getStub.mockImplementation(originalGet);
});

function renderPage(editing: boolean) {
  getStub.mockImplementation(
    async (url: string) =>
      new Msg(
        true,
        '',
        url.endsWith('/proxyChains/list')
          ? editing
            ? [chain]
            : []
          : [
              { id: 6, remark: '新加坡-Titan', protocol: 'vless' },
              { id: 4, remark: '香港-Neburst', protocol: 'vless' },
            ],
      ),
  );
  renderWithProviders(<ChainsPage />);
}

describe('chain form request encoding', () => {
  it('allows an existing target to be selected for another route', async () => {
    renderPage(true);
    await screen.findByText(chain.name);
    fireEvent.click(screen.getByRole('button', { name: /创建转发链/ }));
    const target = screen.getByLabelText('目标节点') as HTMLSelectElement;
    expect(Array.from(target.options).map((option) => option.value)).toContain('6');
  });

  it.each([false, true])('sends typed JSON when editing=%s', async (editing) => {
    renderPage(editing);
    if (editing) {
      await screen.findByText(chain.name);
      fireEvent.click(screen.getByRole('img', { name: 'edit' }).closest('button')!);
    } else {
      await screen.findByText('还没有链式配置');
      fireEvent.click(screen.getByRole('button', { name: /创建转发链/ }));
      fireEvent.change(screen.getByLabelText('链路名称'), { target: { value: chain.name } });
      await screen.findAllByRole('option', { name: '新加坡-Titan · vless' });
      fireEvent.change(screen.getByLabelText('目标节点'), { target: { value: '6' } });
      fireEvent.change(screen.getByLabelText('中转节点'), { target: { value: '4' } });
    }
    fireEvent.change(screen.getByLabelText('中转显示名称'), { target: { value: 'SG via HK' } });
    // false must survive JSON serialization just as the numeric IDs do.
    fireEvent.click(screen.getByRole('switch'));
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(editing ? '/panel/api/proxyChains/update/7' : '/panel/api/proxyChains/add');
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json');
    expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('test-token');
    expect(JSON.parse(String(init?.body))).toEqual({
      name: chain.name,
      targetInboundId: 6,
      relayInboundId: 4,
      enabled: false,
      relayName: 'SG via HK',
    });
    await screen.findByText('链式配置已保存');
  });
});

it('shows a failed relay check without removing the chain', async () => {
  fetchMock.mockImplementation(
    async () =>
      new Response(JSON.stringify({ success: false, msg: '中转到目标：network is unreachable' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
  );
  renderPage(true);
  await screen.findByText(chain.name);
  expect(screen.getByText('未检查（启用不代表可连通）')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: '检查链路' }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(fetchMock.mock.calls[0][0]).toBe('/panel/api/proxyChains/check/7');
  await screen.findAllByText('中转到目标：network is unreachable');
  expect(screen.getByText(chain.name)).toBeTruthy();
});
