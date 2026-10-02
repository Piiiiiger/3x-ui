import { useState } from 'react';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import ClientRenewModal from '@/pages/clients/ClientRenewModal';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

const postStub = vi.mocked(HttpUtil.post);
const setupPost = postStub.getMockImplementation();

afterEach(() => {
  if (setupPost) postStub.mockImplementation(setupPost);
});

function renderModal(emails: string[]) {
  postStub.mockResolvedValue(new Msg(true, '', { affected: emails.length }));
  const onClose = vi.fn();
  renderWithProviders(<ClientRenewModal open emails={emails} onClose={onClose} />);
  return onClose;
}

describe('ClientRenewModal', () => {
  // Renewing is per user now: a month on top by default, with the usage cleared.
  it('renews by 30 days and clears the usage unless told otherwise', async () => {
    const onClose = renderModal(['alice', 'bob']);
    expect(screen.getByRole('dialog', { name: 'Renew 2 user(s)' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Renew' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/clients/renew',
        { emails: ['alice', 'bob'], days: 30, resetUsage: true },
        expect.anything(),
      ),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('renews by the days entered, keeping the usage when unticked', async () => {
    renderModal(['alice']);
    expect(screen.getByText('Renew alice')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Days to add'), { target: { value: '7' } });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Clear used traffic' }));
    fireEvent.click(screen.getByRole('button', { name: 'Renew' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/clients/renew',
        { emails: ['alice'], days: 7, resetUsage: false },
        expect.anything(),
      ),
    );
  });

  // One dialog serves every renewal, so what was typed for one user must not carry over.
  it('starts over once closed', () => {
    function Harness() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button onClick={() => setOpen(true)}>reopen</button>
          <ClientRenewModal open={open} emails={['alice']} onClose={() => setOpen(false)} />
        </>
      );
    }
    renderWithProviders(<Harness />);
    fireEvent.change(screen.getByLabelText('Days to add'), { target: { value: '7' } });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Clear used traffic' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    fireEvent.click(screen.getByText('reopen'));

    expect((screen.getByLabelText('Days to add') as HTMLInputElement).value).toBe('30');
    expect(
      (screen.getByRole('checkbox', { name: 'Clear used traffic' }) as HTMLInputElement).checked,
    ).toBe(true);
  });
});
