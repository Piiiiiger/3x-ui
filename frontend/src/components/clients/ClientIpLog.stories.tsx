import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, waitFor, within } from 'storybook/test';
import { Button } from 'antd';

import type { ClientIpBan, ClientIpInfo } from '@/lib/clients/ip-log';

import ClientIpLogModal from './ClientIpLog';

const meta = {
  title: 'Clients/ClientIpLogModal',
  component: ClientIpLogModal,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component:
          "A client's live source addresses and IP-limit bans. Addresses the limit ignores (Pigger servers, the allowlist, private ranges) are labelled, banned ones show the minutes left, and each ban can be lifted early.",
      },
    },
  },
  argTypes: {
    open: { description: 'Whether the modal is visible.' },
    email: { description: 'Client email shown in the title.' },
    ips: { description: 'Entries from `POST /panel/api/clients/ips/:email`.' },
    bans: { description: 'Running bans from `POST /panel/api/clients/ipBans/:email`.' },
    nowMs: { description: 'When the lists were fetched (ms); minutes left count from it.' },
    unbanning: { description: 'Network whose unban request is in flight, if any.' },
    onUnban: { description: 'Called with a ban network when its Unban button is pressed.' },
  },
} satisfies Meta<typeof ClientIpLogModal>;

export default meta;

type Story = StoryObj<typeof meta>;

const NOW_MS = Date.UTC(2026, 9, 5, 12, 0, 0);
const IN_14_MIN = NOW_MS / 1000 + 14 * 60;

const entry = (ip: string, extra: Partial<ClientIpInfo> = {}): ClientIpInfo => ({
  ip,
  time: '2026-10-05 12:00:00',
  node: 'sg-exit',
  exempt: '',
  exemptHost: '',
  bannedUntil: 0,
  ...extra,
});

const sampleIps: ClientIpInfo[] = [
  entry('198.51.100.20', { node: 'hk-relay' }),
  entry('203.0.113.7', { exempt: 'host', exemptHost: 'hk-relay' }),
  entry('2001:db8:1:2::10'),
  entry('198.51.100.9', { bannedUntil: IN_14_MIN }),
  entry('10.10.16.1', { node: 'se-nat', exempt: 'private' }),
];

const sampleBans: ClientIpBan[] = [{ network: '198.51.100.9', bannedAt: 0, expiresAt: IN_14_MIN }];

function Demo() {
  const [open, setOpen] = useState(false);
  const [bans, setBans] = useState(sampleBans);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Show IP log</Button>
      <ClientIpLogModal
        open={open}
        email="alice"
        ips={sampleIps}
        bans={bans}
        nowMs={NOW_MS}
        loading={false}
        clearing={false}
        unbanning={null}
        onRefresh={() => undefined}
        onClear={() => undefined}
        onUnban={(network) => setBans((prev) => prev.filter((b) => b.network !== network))}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

const placeholderArgs = {
  open: false,
  ips: [],
  bans: [],
  nowMs: 0,
  loading: false,
  clearing: false,
  unbanning: null,
  onRefresh: () => undefined,
  onClear: () => undefined,
  onUnban: () => undefined,
  onClose: () => undefined,
};

export const ExemptAndBanned: Story = {
  args: placeholderArgs,
  render: () => <Demo />,
  play: async ({ canvas, canvasElement, userEvent }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(canvas.getByRole('button', { name: 'Show IP log' }));
    const relay = await body.findByText('Server hk-relay · not counted');
    await waitFor(() => expect(relay).toBeVisible());
    await userEvent.click(body.getByRole('button', { name: 'Unban' }));
    await waitFor(() => expect(body.queryByRole('button', { name: 'Unban' })).toBeNull());
  },
};
