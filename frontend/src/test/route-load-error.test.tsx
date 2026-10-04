import { screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { render } from '@testing-library/react';
import RouteLoadError from '@/components/feedback/RouteLoadError';
it('offers recovery without automatically discarding the page when a chunk fails', async () => {
  const router = createMemoryRouter([
    {
      path: '/',
      loader: () => {
        throw new TypeError('Failed to fetch dynamically imported module: /assets/old.js');
      },
      errorElement: <RouteLoadError />,
    },
  ]);
  render(<RouterProvider router={router} />);
  expect(await screen.findByText('页面资源未能加载')).toBeTruthy();
  expect(screen.getByRole('button', { name: '刷新并恢复页面' })).toBeTruthy();
  expect(screen.getByText(/未保存的输入会丢失/)).toBeTruthy();
});
