import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import PortalLogin from '@/pages/sub/portal/PortalLogin';
import { renderWithProviders } from './test-utils';

afterEach(() => vi.unstubAllGlobals());

function submitAdmin() {
  fireEvent.click(screen.getByRole('button', { name: 'Administrator sign in' }));
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'admin-password' } });
  fireEvent.change(screen.getByLabelText('Authenticator code'), { target: { value: '123456' } });
  fireEvent.click(screen.getByRole('button', { name: 'Administrator sign in' }));
}

describe('administrator sign-in through the portal', () => {
  it('requires enabling two-factor authentication before issuing a session', async () => {
    const request = vi
      .fn()
      .mockResolvedValue(new Response('{"error":"twoFactorRequired"}', { status: 403 }));
    vi.stubGlobal('fetch', request);
    const signedIn = vi.fn();
    renderWithProviders(<PortalLogin base="/x/portal" onSignedIn={signedIn} />);
    submitAdmin();
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'Administrator access requires two-factor authentication.',
      ),
    );
    expect(request).toHaveBeenCalledOnce();
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({
      username: 'admin',
      password: 'admin-password',
      twoFactorCode: '123456',
    });
    expect(signedIn).not.toHaveBeenCalled();
  });

  it('exchanges the handoff at the panel and refuses a failed exchange', async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(
        new Response('{"token":"browser-bound","path":"/portal-admin"}', { status: 200 }),
      )
      .mockResolvedValueOnce(new Response('{"error":"unauthorized"}', { status: 401 }));
    vi.stubGlobal('fetch', request);
    const signedIn = vi.fn();
    renderWithProviders(<PortalLogin base="/x/portal" onSignedIn={signedIn} />);
    submitAdmin();
    await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
    expect(request.mock.calls[0][0]).toBe('/x/portal/admin-login');
    expect(request.mock.calls[1][0]).toBe('/portal-admin');
    expect(JSON.parse(request.mock.calls[1][1].body)).toEqual({ token: 'browser-bound' });
    await waitFor(() => expect(screen.getByRole('alert')).toBeTruthy());
    expect(signedIn).not.toHaveBeenCalled();
  });
});
