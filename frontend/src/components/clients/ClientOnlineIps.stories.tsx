import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, waitFor, within } from 'storybook/test';
import { Button } from 'antd';

import type { ClientOnlineIps } from '@/generated/zod';

import ClientOnlineIpsModal from './ClientOnlineIps';

const meta = {
  title: 'Clients/ClientOnlineIpsModal',
  component: ClientOnlineIpsModal,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component:
          "A client's IP-limit slots in use, the IPs online right now with the servers they connect to, and the IPs banned for going over the limit with the time left. Relay addresses never appear; an allowlisted IP is listed as not counted.",
      },
    },
  },
  argTypes: {
    open: { description: 'Whether the modal is visible.' },
    email: { description: 'Client email shown in the title.' },
    data: { description: 'The answer of `POST /panel/api/clients/onlineIps/:email`.' },
    nowMs: { description: 'When the data was fetched (ms); minutes left count from it.' },
    unbanning: { description: 'Network whose unban request is in flight, if any.' },
    onUnban: { description: 'Called with a ban network when its Unban button is pressed.' },
  },
} satisfies Meta<typeof ClientOnlineIpsModal>;

export default meta;

type Story = StoryObj<typeof meta>;

const NOW_MS = Date.UTC(2026, 9, 5, 12, 0, 0);
const IN_14_MIN = NOW_MS / 1000 + 14 * 60;

const sample: ClientOnlineIps = {
  limit: 3,
  count: 2,
  online: [
    {
      network: '198.51.100.20',
      addresses: ['198.51.100.20'],
      servers: ['HK relay'],
      lastSeen: NOW_MS / 1000 - 5,
      counted: true,
    },
    {
      network: '2001:db8:1:2::/64',
      addresses: ['2001:db8:1:2::10'],
      servers: ['SG exit'],
      lastSeen: NOW_MS / 1000 - 40,
      counted: true,
    },
  ],
  bans: [{ id: 1, email: 'alice', network: '198.51.100.9', bannedAt: 0, expiresAt: IN_14_MIN }],
};

function Demo() {
  const [open, setOpen] = useState(false);
  const [data, setData] = useState(sample);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Show online IPs</Button>
      <ClientOnlineIpsModal
        open={open}
        email="alice"
        data={data}
        nowMs={NOW_MS}
        loading={false}
        unbanning={null}
        onRefresh={() => undefined}
        onUnban={(network) =>
          setData((prev) => ({ ...prev, bans: prev.bans.filter((b) => b.network !== network) }))
        }
        onClose={() => setOpen(false)}
      />
    </>
  );
}

const placeholderArgs = {
  open: false,
  data: null,
  nowMs: 0,
  loading: false,
  unbanning: null,
  onRefresh: () => undefined,
  onUnban: () => undefined,
  onClose: () => undefined,
};

export const SlotsAndBans: Story = {
  args: placeholderArgs,
  render: () => <Demo />,
  play: async ({ canvas, canvasElement, userEvent }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(canvas.getByRole('button', { name: 'Show online IPs' }));
    const slots = await body.findByText('2/3');
    await waitFor(() => expect(slots).toBeVisible());
    await expect(body.getByText('HK relay')).toBeVisible();
    await userEvent.click(body.getByRole('button', { name: 'Unban' }));
    await waitFor(() => expect(body.queryByRole('button', { name: 'Unban' })).toBeNull());
  },
};
