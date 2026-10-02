import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';

import PortalRedeemModal from '@/pages/sub/portal/PortalRedeemModal';
import { renderWithProviders } from './test-utils';

afterEach(() => vi.unstubAllGlobals());

it('applies the typed code and refreshes the signed-in account', async () => {
  const request = vi.fn().mockResolvedValue(new Response('{"success":true}', { status: 200 }));
  vi.stubGlobal('fetch', request);
  const activated = vi.fn();
  renderWithProviders(
    <PortalRedeemModal
      base="/x/portal"
      onActivated={activated}
      onClose={() => {}}
      onSessionEnded={() => {}}
    />,
  );
  fireEvent.change(screen.getByLabelText('Activation code'), {
    target: { value: 'ABCD-EFGH-JKLM-NPQR' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Activate code' }));
  await waitFor(() => expect(activated).toHaveBeenCalledOnce());
  expect(request.mock.calls[0][0]).toBe('/x/portal/redeem');
  expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({ code: 'ABCD-EFGH-JKLM-NPQR' });
});

it('ends an expired session without announcing activation', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response('{"error":"unauthorized"}', { status: 401 })),
  );
  const ended = vi.fn(),
    activated = vi.fn();
  renderWithProviders(
    <PortalRedeemModal
      base="/x/portal"
      onActivated={activated}
      onClose={() => {}}
      onSessionEnded={ended}
    />,
  );
  fireEvent.change(screen.getByLabelText('Activation code'), {
    target: { value: 'ABCD-EFGH-JKLM-NPQR' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Activate code' }));
  await waitFor(() => expect(ended).toHaveBeenCalledOnce());
  expect(activated).not.toHaveBeenCalled();
});
