import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import LoginPage from '@/pages/login/LoginPage';
import { HttpUtil } from '@/utils';
import { renderWithProviders } from './test-utils';

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe('remember me on the panel login', () => {
  it('sends the choice with the sign-in and keeps the box ticked next time', async () => {
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockImplementation(async (url: string) =>
        url === '/getTwoFactorEnable'
          ? { success: true, msg: '', obj: false }
          : { success: true, msg: '', obj: null },
      );
    const first = renderWithProviders(<LoginPage />);
    const box = await screen.findByRole<HTMLInputElement>('checkbox', {
      name: 'Remember me for 30 days',
    });
    expect(box.checked).toBe(false);
    fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'admin' } });
    fireEvent.click(box);
    fireEvent.click(screen.getByRole('button', { name: 'Log In' }));
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/login', {
        username: 'admin',
        password: 'admin',
        twoFactorCode: '',
        rememberMe: true,
      }),
    );
    first.unmount();

    renderWithProviders(<LoginPage />);
    const again = await screen.findByRole<HTMLInputElement>('checkbox', {
      name: 'Remember me for 30 days',
    });
    expect(again.checked).toBe(true);
  });
});
