import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import PortalRules from '@/pages/sub/portal/PortalRules';
import { renderWithProviders } from './test-utils';

const DAY = 24 * 60 * 60 * 1000;

beforeEach(() => {
  localStorage.clear();
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-10-05T12:00:00Z'));
});

afterEach(() => {
  vi.useRealTimers();
});

it('opens on a visit with the location warning first', async () => {
  renderWithProviders(<PortalRules limitIp={3} />);
  const dialog = await screen.findByRole('dialog');
  const warning = screen.getByText('手机使用时请务必关闭定位服务');
  expect(dialog.textContent?.indexOf('定位服务')).toBeLessThan(
    dialog.textContent?.indexOf('禁止') ?? 0,
  );
  expect(warning.closest('[role="alert"]')).not.toBeNull();
  expect(dialog.textContent).toContain('同一时间最多 3 个 IP 在线');
});

it('stays closed for 7 days once snoozed, then opens again', async () => {
  const first = renderWithProviders(<PortalRules limitIp={3} />);
  fireEvent.click(await screen.findByRole('button', { name: '7 天内不再提示' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  first.unmount();

  vi.setSystemTime(new Date(Date.now() + 6 * DAY));
  const sixDaysOn = renderWithProviders(<PortalRules limitIp={3} />);
  expect(screen.queryByRole('dialog')).toBeNull();
  sixDaysOn.unmount();

  vi.setSystemTime(new Date(Date.now() + 2 * DAY));
  renderWithProviders(<PortalRules limitIp={3} />);
  expect(await screen.findByRole('dialog')).not.toBeNull();
});

it('closes for now without snoozing, and the header button opens it again', async () => {
  const first = renderWithProviders(<PortalRules limitIp={3} />);
  fireEvent.click(await screen.findByRole('button', { name: '我知道了' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  fireEvent.click(screen.getByRole('button', { name: /使用须知/ }));
  expect(await screen.findByRole('dialog')).not.toBeNull();
  first.unmount();

  renderWithProviders(<PortalRules limitIp={3} />);
  expect(await screen.findByRole('dialog')).not.toBeNull();
});
