import { fireEvent, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import PortalAccountCode from '@/pages/sub/portal/PortalAccountCode';
import { PortalDataSchema } from '@/schemas/portal';
import { renderWithProviders } from './test-utils';

it('shows the account credential and Telegram settings without a redemption form', async () => {
  const data = PortalDataSchema.parse({
    email: 'owner',
    page: null,
    plan: null,
    daily: [],
    telegramBound: true,
    account: { code: 'ABCD-EFGH-JKLM-NPQR', dailyEnabled: false, dailyTime: '09:30' },
  });
  renderWithProviders(<PortalAccountCode data={data} />);
  fireEvent.click(screen.getByRole('button', { name: '我的激活码' }));
  expect(await screen.findByText('ABCD-EFGH-JKLM-NPQR')).not.toBeNull();
  expect(screen.getByText('Telegram 已绑定')).not.toBeNull();
  expect(screen.getByText(/当前日报：关闭/).textContent).toContain('09:30');
  expect(screen.queryByRole('textbox')).toBeNull();
  expect(screen.queryByText('使用激活码')).toBeNull();
});

it('binds Telegram in one tap: the bot link carries the code until the account is bound', async () => {
  const base = {
    email: 'owner',
    page: null,
    plan: null,
    daily: [],
    telegramBot: 'example_bot',
    account: { code: 'ABCD-EFGH-JKLM-NPQR', dailyEnabled: true, dailyTime: '20:00' },
  };
  const { unmount } = renderWithProviders(
    <PortalAccountCode data={PortalDataSchema.parse({ ...base, telegramBound: false })} />,
  );
  fireEvent.click(screen.getByRole('button', { name: '我的激活码' }));
  const bind = await screen.findByRole('link', { name: /一键绑定 Telegram/ });
  expect(bind.getAttribute('href')).toBe('https://t.me/example_bot?start=ABCD-EFGH-JKLM-NPQR');
  unmount();

  renderWithProviders(
    <PortalAccountCode data={PortalDataSchema.parse({ ...base, telegramBound: true })} />,
  );
  fireEvent.click(screen.getByRole('button', { name: '我的激活码' }));
  const open = await screen.findByRole('link', { name: /打开 Telegram 机器人/ });
  expect(open.getAttribute('href')).toBe('https://t.me/example_bot');
});
