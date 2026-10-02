import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import CloneInboundModal from '@/pages/inbounds/CloneInboundModal';
import { HttpUtil, Msg } from '@/utils';
import { DBInbound } from '@/models/dbinbound';
import { ThemeProvider } from '@/hooks/useTheme';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

import { renderWithProviders } from './test-utils';

const postSpy = vi.mocked(HttpUtil.post);

const NODES = [
  { id: 2, name: 'arm2', enable: true, status: 'online' },
  { id: 3, name: 'arm3', enable: true, status: 'offline' },
  { id: 4, name: 'retired', enable: false, status: 'online' },
  { id: 5, name: 'arm5', enable: true, status: 'unknown' },
] as unknown as NodeRecord[];

function sourceInbound() {
  return new DBInbound({
    id: 7,
    port: 443,
    listen: '',
    protocol: 'vless',
    remark: 'edge',
    enable: true,
    settings: JSON.stringify({ clients: [{ id: 'uuid-1', email: 'a@test' }], decryption: 'none' }),
    streamSettings: JSON.stringify({ network: 'tcp', security: 'none' }),
    sniffing: '',
    nodeId: 2,
    shareAddrStrategy: 'node',
    shareAddr: '',
  });
}

const SHARES = new Map([
  [0, { shareAddrStrategy: 'custom', shareAddr: '198.51.100.19' }],
  [2, { shareAddrStrategy: 'custom', shareAddr: '198.51.100.97' }],
]);

function renderModal(onCloned = vi.fn(), onClose = vi.fn(), source = sourceInbound()) {
  renderWithProviders(
    <CloneInboundModal
      open
      dbInbound={source}
      nodes={NODES}
      portsInUse={new Map([[2, new Set([443])]])}
      sharesByHost={SHARES}
      onClose={onClose}
      onCloned={onCloned}
    />,
  );
  return { onCloned, onClose };
}

function openTargetDropdown() {
  // antd v6 Select has no .ant-select-selector; mouseDown on the root opens it.
  const selector = document.querySelector('.ant-select');
  if (!selector) throw new Error('target select not rendered');
  fireEvent.mouseDown(selector);
}

function clickOption(text: string) {
  const option = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
    (o) => (o.textContent ?? '').trim() === text,
  );
  if (!option) throw new Error(`option '${text}' not found`);
  fireEvent.click(option);
}

function clickOk() {
  fireEvent.click(screen.getByRole('button', { name: 'Clone' }));
}

type PostBody = Record<string, unknown> & { nodeId?: number };
const postedBodies = () => postSpy.mock.calls.map((c) => c[1] as PostBody);

const selectedTitles = () =>
  Array.from(document.querySelectorAll('.ant-select-selection-item[title]')).map((el) =>
    el.getAttribute('title'),
  );

beforeEach(() => {
  postSpy.mockClear();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  postSpy.mockResolvedValue({ success: true, obj: {} } as any);
});

describe('CloneInboundModal', () => {
  it('pre-selects the source node and clones onto it with a fresh port and no clients', async () => {
    const { onCloned, onClose } = renderModal();

    expect(document.querySelector('.ant-select-selection-item[title="arm2"]')).toBeTruthy();

    clickOk();
    await waitFor(() => expect(postSpy).toHaveBeenCalledTimes(1));

    expect(postSpy.mock.calls[0][0]).toBe('/panel/api/inbounds/add');
    const body = postedBodies()[0];
    expect(body.nodeId).toBe(2);
    expect(body.enable).toBe(false);
    expect(body.remark).toBe('edge (clone)');
    expect(body.port).not.toBe(443);
    expect(body).not.toHaveProperty('tag');
    expect(JSON.parse(body.settings as string).clients).toEqual([]);

    await waitFor(() => expect(onCloned).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('posts once per selected target and omits nodeId for the local panel', async () => {
    renderModal();
    openTargetDropdown();
    clickOption('Local panel');

    clickOk();
    await waitFor(() => expect(postSpy).toHaveBeenCalledTimes(2));

    const [nodeBody, localBody] = postedBodies();
    expect(nodeBody.nodeId).toBe(2);
    expect(localBody).not.toHaveProperty('nodeId');
    expect(nodeBody.port).not.toBe(443);
  });

  it('disables non-online nodes and hides disabled nodes from the target list', () => {
    renderModal();
    openTargetDropdown();

    const option = (text: string) =>
      Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => (o.textContent ?? '').trim() === text,
      );
    // Only `online` is selectable — `offline` and `unknown` (no heartbeat
    // yet) are both shown but disabled.
    expect(option('arm3 (offline)')?.className).toContain('ant-select-item-option-disabled');
    expect(option('arm5 (unknown)')?.className).toContain('ant-select-item-option-disabled');
    expect(option('arm2')?.className).not.toContain('ant-select-item-option-disabled');

    const labels = Array.from(document.querySelectorAll('.ant-select-item-option')).map((o) =>
      (o.textContent ?? '').trim(),
    );
    expect(labels).toEqual(['Local panel', 'arm2', 'arm3 (offline)', 'arm5 (unknown)']);
  });

  it('select-all picks only selectable targets and clear-all blocks submit', () => {
    renderModal();

    const selectAll = screen.getByRole('button', { name: 'Select all' });
    fireEvent.click(selectAll);

    // Local panel + online node; offline/unknown nodes stay unpickable.
    expect(selectedTitles().sort()).toEqual(['Local panel', 'arm2']);
    expect((selectAll as HTMLButtonElement).disabled).toBe(true);

    // Clear all empties the selection and disables OK.
    fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    expect(selectedTitles()).toEqual([]);
    expect((screen.getByRole('button', { name: 'Clone' }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it('keeps a cleared selection when the nodes list refetches mid-dialog', () => {
    // The page LazyMounts the modal once and keeps it mounted; heartbeats give
    // `nodes` a new array identity on every refetch. The reset effect must not
    // refire on that — only on the open transition.
    const modal = (nodes: NodeRecord[], open = true) => (
      <ThemeProvider>
        <CloneInboundModal
          open={open}
          dbInbound={sourceInbound()}
          nodes={nodes}
          portsInUse={new Map()}
          sharesByHost={SHARES}
          onClose={() => {}}
          onCloned={() => {}}
        />
      </ThemeProvider>
    );
    const { rerender } = render(modal(NODES));

    fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    expect(selectedTitles()).toEqual([]);

    rerender(modal(NODES.map((n) => ({ ...n, latencyMs: 42 })) as unknown as NodeRecord[]));
    expect(selectedTitles()).toEqual([]);
  });

  it('resets the selection to the source node on each reopen', () => {
    const modal = (open: boolean) => (
      <ThemeProvider>
        <CloneInboundModal
          open={open}
          dbInbound={sourceInbound()}
          nodes={NODES}
          portsInUse={new Map()}
          sharesByHost={SHARES}
          onClose={() => {}}
          onCloned={() => {}}
        />
      </ThemeProvider>
    );
    const { rerender } = render(modal(true));

    fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    expect(selectedTitles()).toEqual([]);

    rerender(modal(false));
    rerender(modal(true));
    expect(selectedTitles()).toEqual(['arm2']);
  });

  it('reports a partial failure with the backend reason and still closes', async () => {
    const { onCloned, onClose } = renderModal();
    postSpy.mockImplementation(async (_url, data) => {
      const body = data as PostBody;
      if (body.nodeId === 2) {
        return {
          success: false,
          msg: "port 23456 (tcp) already used by inbound 'x' (#1) on *",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
        } as any;
      }
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return { success: true, obj: {} } as any;
    });

    openTargetDropdown();
    clickOption('Local panel');
    clickOk();

    await screen.findByText(/port 23456 \(tcp\) already used/);
    await waitFor(() => expect(onCloned).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  describe('a REALITY node', () => {
    function realitySource() {
      const src = sourceInbound();
      src.shareAddrStrategy = 'custom';
      src.shareAddr = '198.51.100.97';
      src.streamSettings = JSON.stringify({
        network: 'tcp',
        security: 'reality',
        realitySettings: {
          target: 'www.bing.com:443',
          serverNames: ['www.bing.com'],
          privateKey: 'src-priv',
          shortIds: ['aa'],
          settings: { publicKey: 'src-pub', spiderX: '/src' },
        },
      });
      return src;
    }

    const getSpy = vi.mocked(HttpUtil.get);
    let restore: ReturnType<typeof getSpy.getMockImplementation>;
    beforeEach(() => {
      restore = getSpy.getMockImplementation();
    });
    afterEach(() => {
      if (restore) getSpy.mockImplementation(restore);
    });

    it("gives every copy its own keys and its target host's address", async () => {
      let issued = 0;
      getSpy.mockImplementation(async (url: string) => {
        if (url !== '/panel/api/server/getNewX25519Cert') return new Msg(true, '', {});
        issued++;
        return new Msg(true, '', { privateKey: `priv-${issued}`, publicKey: `pub-${issued}` });
      });
      renderModal(vi.fn(), vi.fn(), realitySource());
      openTargetDropdown();
      clickOption('Local panel');

      clickOk();
      await waitFor(() => expect(postSpy).toHaveBeenCalledTimes(2));

      const [sameHost, local] = postedBodies().map((b) => ({
        reality: JSON.parse(b.streamSettings as string).realitySettings,
        shareAddr: b.shareAddr,
      }));
      expect([sameHost.reality.privateKey, local.reality.privateKey]).toEqual(['priv-1', 'priv-2']);
      expect([sameHost.reality.settings.publicKey, local.reality.settings.publicKey]).toEqual([
        'pub-1',
        'pub-2',
      ]);
      expect(sameHost.reality.shortIds).not.toEqual(['aa']);
      expect(local.reality.target).toBe('www.bing.com:443');
      expect([sameHost.shareAddr, local.shareAddr]).toEqual(['198.51.100.97', '198.51.100.19']);
    });

    it('posts no copy whose keys could not be made', async () => {
      getSpy.mockImplementation(async () => new Msg(false, 'xray x25519 failed'));
      renderModal(vi.fn(), vi.fn(), realitySource());

      clickOk();
      await waitFor(() => expect(screen.getByText(/xray x25519 failed/)).toBeTruthy());
      expect(postSpy).not.toHaveBeenCalled();
    });
  });
});
