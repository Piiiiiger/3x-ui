import { useState } from 'react';
import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { ProbeLinkView, ProbeServer } from '@/generated/zod';
import ProbeLinksModal from '@/pages/probe/ProbeLinksModal';
import { HttpUtil, Msg } from '@/utils';
import { keys } from '@/api/queryKeys';
import { chooseSelectOption, makeTestQueryClient, renderWithProviders } from './test-utils';

function liteServer(id: string, name: string, region: string): ProbeServer {
  return {
    id,
    name,
    region,
    os: '',
    arch: '',
    virtualization: '',
    cpuCores: 0,
    status: 'unknown',
    updatedAt: 0,
    cpu: 0,
    memUsed: 0,
    memTotal: 0,
    diskUsed: 0,
    diskTotal: 0,
    load1: 0,
    load5: 0,
    load15: 0,
    netIn: 0,
    netOut: 0,
    netTotalUp: 0,
    netTotalDown: 0,
    uptime: 0,
    trafficLimit: 0,
    trafficUsed: 0,
    pings: [],
    linked: false,
    nodeId: 0,
    nodeName: '',
  };
}

const servers = [
  liteServer('uuid-la', '洛杉矶-Alpha', '🇺🇸'),
  liteServer('uuid-hk', '香港-Bravo', '🇭🇰'),
  liteServer('uuid-uk', '英国-Charlie', ''),
];

// The panel itself, a node whose server Lite dropped, and a node not linked yet.
const stored: ProbeLinkView[] = [
  { nodeId: 0, nodeName: '', address: '', serverId: 'uuid-la', serverName: '洛杉矶-Alpha' },
  { nodeId: 2, nodeName: 'edge-hk', address: '203.0.113.7', serverId: 'uuid-gone', serverName: '' },
  { nodeId: 5, nodeName: 'oracle', address: 'oracle.example.com', serverId: '', serverName: '' },
];

const MISSING = 'Lite no longer lists this server';
const JSON_BODY = { headers: { 'Content-Type': 'application/json' } };

function serveLinks(answer: Msg<unknown> = new Msg(true, '', stored)) {
  vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) =>
    url === '/panel/api/probe/links' ? answer : new Msg(false, `unexpected GET ${url}`),
  );
}

async function renderModal(listed: ProbeServer[] | null = servers) {
  const onClose = vi.fn();
  const onSaved = vi.fn();
  renderWithProviders(
    <ProbeLinksModal open servers={listed} onClose={onClose} onSaved={onSaved} />,
  );
  await screen.findByText('oracle.example.com');
  return { onClose, onSaved };
}

function selectOf(nodeId: number): HTMLElement {
  const select = document.getElementById(`probe-link-${nodeId}`)?.closest('.ant-select');
  if (!(select instanceof HTMLElement)) throw new Error(`no select for node ${nodeId}`);
  return select;
}

function rowOf(nodeId: number): HTMLElement {
  return selectOf(nodeId).closest('.ant-form-item') as HTMLElement;
}

// Opens the dropdown and maps each option to whether it can be picked.
function optionsOf(nodeId: number): Record<string, boolean> {
  const select = selectOf(nodeId);
  fireEvent.mouseDown(select.querySelector('.ant-select-selector') ?? select);
  const options = Array.from(
    document.querySelectorAll(
      '.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option',
    ),
  );
  const pickable = Object.fromEntries(
    options.map((option) => [
      option.getAttribute('title') ?? '',
      !option.classList.contains('ant-select-item-option-disabled'),
    ]),
  );
  fireEvent.keyDown(select, { key: 'Escape' });
  return pickable;
}

function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
}

// The links query answering again under the open form, as on a window focus.
async function askAgain(queryClient: QueryClient) {
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: keys.probe.links() });
    // The query tells its observers on the next macrotask.
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ProbeLinksModal', () => {
  it('lists the panel and every node with the server each one is linked to', async () => {
    serveLinks();
    await renderModal();

    expect(rowOf(0).textContent).toContain('Local panel');
    expect(rowOf(0).textContent).toContain('🇺🇸 洛杉矶-Alpha');
    expect(rowOf(2).textContent).toContain('edge-hk');
    expect(rowOf(2).textContent).toContain('203.0.113.7');
    expect(rowOf(5).textContent).toContain('oracle');
    expect(rowOf(5).textContent).toContain('Not linked');
  });

  // Two hosts on one server is refused by the API: the form does not offer it.
  it('offers a server to the host that has it and to no other', async () => {
    serveLinks();
    await renderModal();

    expect(optionsOf(5)).toEqual({
      '🇺🇸 洛杉矶-Alpha': false,
      '🇭🇰 香港-Bravo': true,
      '英国-Charlie': true,
    });
    expect(optionsOf(0)['🇺🇸 洛杉矶-Alpha']).toBe(true);
  });

  // Left to itself the select searches the option values, which here are uuids.
  it('finds a server by its name as the admin types', async () => {
    serveLinks();
    await renderModal();
    const select = selectOf(5);

    fireEvent.mouseDown(select.querySelector('.ant-select-selector') ?? select);
    fireEvent.change(document.getElementById('probe-link-5') as HTMLInputElement, {
      target: { value: 'bravo' },
    });

    const shown = Array.from(
      document.querySelectorAll(
        '.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option',
      ),
    ).map((option) => option.getAttribute('title'));
    expect(shown).toEqual(['🇭🇰 香港-Bravo']);
  });

  it('flags a link whose server Lite no longer lists, and only that one', async () => {
    serveLinks();
    await renderModal();

    expect(rowOf(2).textContent).toContain(MISSING);
    expect(screen.getAllByText(MISSING)).toHaveLength(1);
  });

  // With Lite down every link would look orphaned, and clearing them would lose them.
  it('flags nothing while the list of Lite servers is not known', async () => {
    serveLinks();
    await renderModal(null);

    expect(screen.queryByText(MISSING)).toBeNull();
    expect(
      screen.getByText('Lite cannot be read right now, so its servers cannot be listed.'),
    ).toBeTruthy();
  });

  // The API replaces the stored set, so every link is sent, changed or not.
  it('saves the whole set of links as JSON and reports the save', async () => {
    serveLinks();
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const { onSaved } = await renderModal();

    chooseSelectOption('probe-link-5', '🇭🇰 香港-Bravo');
    save();

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(post.mock.calls).toEqual([
      [
        '/panel/api/probe/links',
        {
          links: [
            { nodeId: 0, serverId: 'uuid-la' },
            { nodeId: 2, serverId: 'uuid-gone' },
            { nodeId: 5, serverId: 'uuid-hk' },
          ],
        },
        JSON_BODY,
      ],
    ]);
  });

  it('leaves a cleared host out of the set, which unlinks it', async () => {
    serveLinks();
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const { onSaved } = await renderModal();

    const clear = selectOf(2).querySelector('.ant-select-clear');
    if (!clear) throw new Error('the select of node 2 cannot be cleared');
    fireEvent.mouseDown(clear);
    fireEvent.click(clear);
    await waitFor(() => expect(rowOf(2).textContent).not.toContain(MISSING));
    save();

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(post.mock.calls).toEqual([
      ['/panel/api/probe/links', { links: [{ nodeId: 0, serverId: 'uuid-la' }] }, JSON_BODY],
    ]);
  });

  it('stays open when the server refuses the links', async () => {
    serveLinks();
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockResolvedValue(new Msg(false, 'Something went wrong (node 5 does not exist)'));
    const { onSaved } = await renderModal();

    save();

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' }).className).not.toContain(
        'ant-btn-loading',
      ),
    );
    expect(onSaved).not.toHaveBeenCalled();
  });

  // A form seeded from the last answer would save what was stored then over what is stored now.
  it('reads the stored links again each time it opens', async () => {
    let answer = stored;
    vi.spyOn(HttpUtil, 'get').mockImplementation(async () => new Msg(true, '', answer));
    function Reopenable() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button onClick={() => setOpen(true)}>reopen</button>
          <ProbeLinksModal
            open={open}
            servers={servers}
            onClose={() => setOpen(false)}
            onSaved={() => {}}
          />
        </>
      );
    }
    // The panel's client keeps an answer fresh for 30 s; the test default of 0
    // would refetch on every mount and hide a form that reuses the cache.
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    renderWithProviders(<Reopenable />, { queryClient });
    await screen.findByText('oracle.example.com');
    chooseSelectOption('probe-link-5', '🇭🇰 香港-Bravo');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    answer = [{ ...stored[0], serverId: 'uuid-uk', serverName: '英国-Charlie' }, stored[2]];
    fireEvent.click(screen.getByRole('button', { name: 'reopen' }));

    await waitFor(() => expect(rowOf(0).textContent).toContain('英国-Charlie'));
    expect(rowOf(5).textContent).toContain('Not linked');
    expect(document.getElementById('probe-link-2')).toBeNull();
  });

  // The query answers again on every window focus; a node deleted meanwhile
  // must not shift the hosts under the selects the admin is working in.
  it('keeps every host on its own row when the stored list changes under the open form', async () => {
    let answer = stored;
    vi.spyOn(HttpUtil, 'get').mockImplementation(async () => new Msg(true, '', answer));
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const queryClient = makeTestQueryClient();
    renderWithProviders(
      <ProbeLinksModal open servers={servers} onClose={() => {}} onSaved={() => {}} />,
      { queryClient },
    );
    await screen.findByText('oracle.example.com');

    answer = [stored[0], stored[2]];
    await askAgain(queryClient);
    chooseSelectOption('probe-link-5', '🇭🇰 香港-Bravo');
    save();

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(post.mock.calls[0][1]).toEqual({
      links: [
        { nodeId: 0, serverId: 'uuid-la' },
        { nodeId: 2, serverId: 'uuid-gone' },
        { nodeId: 5, serverId: 'uuid-hk' },
      ],
    });
  });

  // Asked again while the panel was briefly unreachable, the error took the
  // place of the form, and the choices made in it were gone when it came back.
  it('keeps the form and its unsaved choices when a later request for the links fails', async () => {
    let failing = false;
    vi.spyOn(HttpUtil, 'get').mockImplementation(async () =>
      failing ? new Msg(false, 'Request failed') : new Msg(true, '', stored),
    );
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const queryClient = makeTestQueryClient();
    renderWithProviders(
      <ProbeLinksModal open servers={servers} onClose={() => {}} onSaved={() => {}} />,
      { queryClient },
    );
    await screen.findByText('oracle.example.com');
    chooseSelectOption('probe-link-5', '🇭🇰 香港-Bravo');

    failing = true;
    await askAgain(queryClient);

    expect(screen.queryByText('Request failed')).toBeNull();
    save();
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(post.mock.calls[0][1]).toEqual({
      links: [
        { nodeId: 0, serverId: 'uuid-la' },
        { nodeId: 2, serverId: 'uuid-gone' },
        { nodeId: 5, serverId: 'uuid-hk' },
      ],
    });
  });

  // Saved back, a row the form misread would link the wrong host.
  it('builds no form from an answer it does not understand', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    serveLinks(new Msg(true, '', [{ nodeId: 2, serverId: 'uuid-hk' }]));
    renderWithProviders(
      <ProbeLinksModal open servers={servers} onClose={() => {}} onSaved={() => {}} />,
    );

    expect(await screen.findByText('probe/links response failed validation')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
  });

  // Saving the empty list it would show instead would unlink every host.
  it('cannot save over links it could not read', async () => {
    serveLinks(new Msg(false, 'Something went wrong (database is locked)'));
    const post = vi.spyOn(HttpUtil, 'post');
    renderWithProviders(
      <ProbeLinksModal open servers={servers} onClose={() => {}} onSaved={() => {}} />,
    );

    expect(await screen.findByText('Something went wrong (database is locked)')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
    expect(post).not.toHaveBeenCalled();
  });
});
