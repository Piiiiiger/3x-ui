import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import PortalLogin from '@/pages/sub/portal/PortalLogin';
import { renderWithProviders } from './test-utils';

afterEach(() => vi.unstubAllGlobals());

describe('portal registration', () => {
  it('registers with an activation code and enters the signed-in view', async () => {
    const request = vi.fn().mockResolvedValue(new Response('{"success":true}', { status: 200 }));
    vi.stubGlobal('fetch', request);
    const signedIn = vi.fn();
    renderWithProviders(<PortalLogin base="/x/portal" onSignedIn={signedIn} />);
    fireEvent.click(screen.getByRole('button', { name: 'Register' }));
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'newbie' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret-pass' } });
    fireEvent.change(screen.getByLabelText('Activation code'), {
      target: { value: 'ABCD-EFGH-JKLM-NPQR' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Register' }));
    await waitFor(() => expect(signedIn).toHaveBeenCalledOnce());
    expect(request.mock.calls[0][0]).toBe('/x/portal/register');
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({
      username: 'newbie',
      password: 'secret-pass',
      code: 'ABCD-EFGH-JKLM-NPQR',
    });
  });

  it('keeps the form open and explains that the code is invalid', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('{"error":"code"}', { status: 400 })),
    );
    const signedIn = vi.fn();
    renderWithProviders(<PortalLogin base="/x/portal" onSignedIn={signedIn} />);
    fireEvent.click(screen.getByRole('button', { name: 'Register' }));
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'newbie' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret-pass' } });
    fireEvent.change(screen.getByLabelText('Activation code'), {
      target: { value: 'AAAA-BBBB-CCCC-DDDD' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Register' }));
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'The activation code is invalid or already used.',
      ),
    );
    expect(signedIn).not.toHaveBeenCalled();
  });
});
