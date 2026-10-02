import { act, fireEvent, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';

import AppNav from '@/layouts/AppNav';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: {} }),
}));

afterEach(() => {
  vi.restoreAllMocks();
});

// rc-menu registers its items in a microtask after render; settle it inside act().
async function renderNav(path = '/') {
  const view = renderWithProviders(
    <MemoryRouter initialEntries={[path]}>
      <AppNav />
    </MemoryRouter>,
  );
  await act(async () => {});
  return view;
}

function narrowViewport() {
  const real = window.matchMedia;
  vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => ({
    ...real(query),
    matches: query.includes('max-width'),
  }));
}

test('marks the page the router is on as the current top-bar item', async () => {
  await renderNav('/clients');
  const nav = screen.getByRole('menu');

  expect(within(nav).getByText('Clients').closest('li')?.className).toContain(
    'ant-menu-item-selected',
  );
  expect(within(nav).getByText('Overview').closest('li')?.className).not.toContain(
    'ant-menu-item-selected',
  );
});

test('lights up the more button while on a page it holds', async () => {
  await renderNav('/api-docs');
  expect(screen.getByRole('button', { name: 'more' }).className).toContain('is-active');
});

test('leaves the more button plain on the pages the bar shows directly', async () => {
  await renderNav('/inbounds');
  expect(screen.getByRole('button', { name: 'more' }).className).not.toContain('is-active');
});

test('labels the palette shortcut with the modifier the platform actually uses', async () => {
  const view = await renderNav();
  const chip = view.container.querySelector('.app-nav .nav-search-kbd');
  expect(chip?.textContent).toBe('CtrlK');
});

test('collapses the page buttons into the drawer on narrow screens', async () => {
  narrowViewport();
  await renderNav('/inbounds');

  expect(screen.queryByRole('menu')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
  await act(async () => {});

  const drawerNav = document.querySelector('.drawer-nav') as HTMLElement;
  expect(within(drawerNav).getByText('API Docs')).not.toBeNull();
  expect(within(drawerNav).getByText('Inbounds').closest('li')?.className).toContain(
    'ant-menu-item-selected',
  );
});

test("keeps the hosts item lit on one host's own page", async () => {
  await renderNav('/nodes/3');
  const nav = screen.getByRole('menu');
  expect(within(nav).getByText('Hosts').closest('li')?.className).toContain(
    'ant-menu-item-selected',
  );
});
