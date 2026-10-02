import { useState } from 'react';
import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { keys } from '@/api/queryKeys';
import ProbeSettingsModal from '@/pages/probe/ProbeSettingsModal';
import { HttpUtil, Msg } from '@/utils';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

const stored = { url: 'http://127.0.0.1:27777', publicUrl: 'https://probe.example.com' };

function serveSettings(answer: Msg<unknown> = new Msg(true, '', stored)) {
  vi.spyOn(HttpUtil, 'get').mockImplementation(async (url: string) =>
    url === '/panel/api/probe/settings' ? answer : new Msg(false, `unexpected GET ${url}`),
  );
}

// The panel's client keeps an answer fresh for 30 s; the test default of 0 would
// refetch on every mount and hide a form that reuses the cache.
function appLikeQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000 } } });
}

function renderModal() {
  const onClose = vi.fn();
  const onSaved = vi.fn();
  renderWithProviders(<ProbeSettingsModal open onClose={onClose} onSaved={onSaved} />);
  return { onClose, onSaved };
}

async function field(label: string): Promise<HTMLInputElement> {
  return (await screen.findByLabelText(label)) as HTMLInputElement;
}

function type(input: HTMLInputElement, value: string) {
  fireEvent.change(input, { target: { value } });
}

function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ProbeSettingsModal', () => {
  it('opens on the stored addresses', async () => {
    serveSettings();
    renderModal();

    expect((await field('Lite address')).value).toBe('http://127.0.0.1:27777');
    expect((await field('Public page URL')).value).toBe('https://probe.example.com');
  });

  // The handlers bind JSON only: a form-encoded body is refused and nothing is saved.
  it('posts both addresses as JSON and reports the save', async () => {
    serveSettings(new Msg(true, '', { url: '', publicUrl: '' }));
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const { onSaved } = renderModal();

    type(await field('Lite address'), '  http://127.0.0.1:27777 ');
    type(await field('Public page URL'), 'https://probe.example.com');
    save();

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(post.mock.calls).toEqual([
      ['/panel/api/probe/settings', stored, { headers: { 'Content-Type': 'application/json' } }],
    ]);
  });

  it('turns a wrong address away before asking the server', async () => {
    serveSettings();
    const post = vi.spyOn(HttpUtil, 'post');
    const { onSaved } = renderModal();

    type(await field('Lite address'), 'http://192.168.1.10:27777');
    type(await field('Public page URL'), 'probe.example.com');
    save();

    expect(await screen.findByText('This is not a loopback address')).toBeTruthy();
    expect(screen.getByText('This is not an http or https URL')).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it('stays open when the server refuses the addresses', async () => {
    serveSettings();
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockResolvedValue(new Msg(false, 'Something went wrong (refused)'));
    const { onSaved } = renderModal();

    await field('Lite address');
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
  it('reads the stored addresses again each time it opens', async () => {
    let answer = stored;
    vi.spyOn(HttpUtil, 'get').mockImplementation(async () => new Msg(true, '', answer));
    function Reopenable() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button onClick={() => setOpen(true)}>reopen</button>
          <ProbeSettingsModal open={open} onClose={() => setOpen(false)} onSaved={() => {}} />
        </>
      );
    }
    renderWithProviders(<Reopenable />, { queryClient: appLikeQueryClient() });
    expect((await field('Lite address')).value).toBe('http://127.0.0.1:27777');

    type(await field('Lite address'), 'http://127.0.0.1:1');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    answer = { url: 'http://127.0.0.1:28888', publicUrl: '' };
    fireEvent.click(screen.getByRole('button', { name: 'reopen' }));

    await waitFor(async () => expect((await field('Lite address')).value).toBe(answer.url));
  });

  // The cache keeps the last failure next to the last good answer.
  it('does not show the failure of an earlier opening while it reads again', async () => {
    let answer: () => Promise<Msg<unknown>> = async () => new Msg(true, '', stored);
    vi.spyOn(HttpUtil, 'get').mockImplementation(() => answer());
    function Reopenable() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button onClick={() => setOpen(true)}>reopen</button>
          <ProbeSettingsModal open={open} onClose={() => setOpen(false)} onSaved={() => {}} />
        </>
      );
    }
    const closeAndReopen = () => {
      fireEvent.click(screen.getByRole('button', { name: 'Close' }));
      fireEvent.click(screen.getByRole('button', { name: 'reopen' }));
    };
    renderWithProviders(<Reopenable />);
    await field('Lite address');

    answer = async () => new Msg(false, 'Something went wrong (database is locked)');
    closeAndReopen();
    await screen.findByText('Something went wrong (database is locked)');

    let finish: (msg: Msg<unknown>) => void = () => {};
    answer = () => new Promise((resolve) => (finish = resolve));
    closeAndReopen();
    await waitFor(() =>
      expect(screen.queryByText('Something went wrong (database is locked)')).toBeNull(),
    );
    expect(screen.queryByLabelText('Lite address')).toBeNull();

    await act(async () => finish(new Msg(true, '', stored)));
    expect((await field('Lite address')).value).toBe('http://127.0.0.1:27777');
  });

  // Asked again while the panel was briefly unreachable, the error took the
  // place of the form, and what had been typed was gone when it came back.
  it('keeps the form and what was typed when a later request for the settings fails', async () => {
    let failing = false;
    vi.spyOn(HttpUtil, 'get').mockImplementation(async () =>
      failing ? new Msg(false, 'Request failed') : new Msg(true, '', stored),
    );
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', stored));
    const queryClient = makeTestQueryClient();
    renderWithProviders(<ProbeSettingsModal open onClose={() => {}} onSaved={() => {}} />, {
      queryClient,
    });
    type(await field('Lite address'), 'http://127.0.0.1:28888');

    failing = true;
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: keys.probe.settings() });
      // The query tells its observers on the next macrotask.
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(screen.queryByText('Request failed')).toBeNull();
    save();
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(post.mock.calls[0][1]).toEqual({
      url: 'http://127.0.0.1:28888',
      publicUrl: 'https://probe.example.com',
    });
  });

  // Saved back, a field the form misread would overwrite the stored address.
  it('builds no form from an answer it does not understand', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    serveSettings(new Msg(true, '', { url: 'http://127.0.0.1:27777' }));
    renderModal();

    expect(await screen.findByText('probe/settings response failed validation')).toBeTruthy();
    expect(screen.queryByLabelText('Lite address')).toBeNull();
  });

  // Saving the empty form it would show instead would switch the probe off.
  it('cannot save over settings it could not read', async () => {
    serveSettings(new Msg(false, 'Something went wrong (database is locked)'));
    const post = vi.spyOn(HttpUtil, 'post');
    renderModal();

    expect(await screen.findByText('Something went wrong (database is locked)')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
    expect(screen.queryByLabelText('Lite address')).toBeNull();
    expect(post).not.toHaveBeenCalled();
  });
});
