import type { Meta, StoryObj } from '@storybook/react-vite';
import { ConfigProvider, Tag } from 'antd';

import ProbeServerCard, { type ProbeCardServer } from './ProbeServerCard';

const GiB = 1024 ** 3;

const online: ProbeCardServer = {
  name: '新加坡-Delta',
  region: '🇸🇬',
  status: 'online',
  updatedAt: 1790942400000,
  cpu: 12.5,
  memUsed: 0.8 * GiB,
  memTotal: 2 * GiB,
  diskUsed: 8 * GiB,
  diskTotal: 40 * GiB,
  load1: 0.31,
  load5: 0.22,
  load15: 0.18,
  netIn: 5_678_000,
  netOut: 1_234_000,
  netTotalUp: 120 * GiB,
  netTotalDown: 310 * GiB,
  uptime: 19 * 86400 + 5 * 3600,
  pings: [
    { id: 8, name: '电信', latency: 31, loss: 0.4 },
    { id: 9, name: '联通', latency: 126, loss: 8 },
    { id: 10, name: '移动', latency: -1, loss: 100 },
  ],
};

// A server that is not online arrives with every figure zeroed.
const silent: ProbeCardServer = {
  ...online,
  cpu: 0,
  memUsed: 0,
  memTotal: 0,
  diskUsed: 0,
  diskTotal: 0,
  load1: 0,
  load5: 0,
  load15: 0,
  netIn: 0,
  netOut: 0,
  netTotalUp: 0,
  netTotalDown: 0,
  uptime: 0,
  pings: [],
};

const meta = {
  title: 'Probe/ProbeServerCard',
  component: ProbeServerCard,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div className="probe-grid" style={{ maxWidth: 420 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'One monitored server: its flag and name, one status (online, offline, unknown or unmonitored), and for an online server its CPU, memory and disk bars, load, uptime, speeds, traffic totals and, under a network quality heading, the latency of each ping route over a bar of the pings answered. The admin Probe page and the client portal share it, so it fetches nothing and styles itself with theme tokens only. Lay cards out with the `probe-grid` class that ships with it.',
      },
    },
  },
  argTypes: {
    server: {
      description:
        'What to draw. Figures are read only while `status` is `online`. `updatedAt` (unix ms) is the last report of an offline server. A ping `latency` of -1 means no reply in the last hour; its `loss`, a percentage, is the unfilled part of the bar under it. `region` is a flag emoji or a two-letter country code.',
    },
    subtitle: { description: 'Optional line under the name, e.g. OS and architecture.' },
    footer: {
      description: 'Optional block under a divider, e.g. the linked node and a quota bar.',
    },
  },
} satisfies Meta<typeof ProbeServerCard>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Online: Story = {
  args: { server: online },
};

export const Offline: Story = {
  args: { server: { ...silent, name: '香港-Echo', region: '🇭🇰', status: 'offline' } },
};

export const Unknown: Story = {
  args: {
    server: { ...silent, name: '美国-Foxtrot', region: '🇺🇸', status: 'unknown', updatedAt: 0 },
  },
};

export const Unmonitored: Story = {
  args: {
    server: { ...silent, name: '日本-IIJ', region: '', status: 'unmonitored', updatedAt: 0 },
  },
};

export const HighUsage: Story = {
  args: {
    server: {
      ...online,
      name: '洛杉矶-Alpha',
      region: '🇺🇸',
      cpu: 96.4,
      memUsed: 1.7 * GiB,
      diskUsed: 38.5 * GiB,
      load1: 4.82,
      load5: 3.9,
      load15: 2.75,
    },
    subtitle: 'Debian GNU/Linux 13 (trixie) · amd64 · kvm · 1-core',
    footer: <Tag>Local panel</Tag>,
  },
};

export const RightToLeft: Story = {
  args: {
    server: {
      ...online,
      name: 'تهران ۱',
      region: '🇮🇷',
      pings: [{ id: 3, name: 'مخابرات', latency: 58, loss: 2.5 }],
    },
  },
  decorators: [
    (Story) => (
      <ConfigProvider direction="rtl">
        <div dir="rtl">
          <Story />
        </div>
      </ConfigProvider>
    ),
  ],
};
