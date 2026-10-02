import type { Meta, StoryObj } from '@storybook/react-vite';

import RainbowBar from './RainbowBar';

const meta = {
  title: 'UI/RainbowBar',
  component: RainbowBar,
  tags: ['autodocs'],
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          "妙妙屋X's traffic bar for quotas and usage: a rainbow flowing through the used share of a tinted track. The rainbow spans the whole track and is clipped to the share, so its colours keep one size. Without a limit the bar runs full and states no share.",
      },
    },
  },
  argTypes: {
    percent: {
      description:
        'The share used, 0–100 (larger values fill the bar); `null` when there is no limit.',
    },
    label: { description: 'What the bar measures, read out with the value, e.g. "Traffic quota".' },
    valueText: {
      description:
        'The figures in words, e.g. "25.00 GB / 100.00 GB"; read out in place of a share without a limit.',
    },
    className: { description: 'Extra CSS class for the track.' },
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RainbowBar>;

export default meta;

type Story = StoryObj<typeof meta>;

export const QuarterUsed: Story = {
  args: { percent: 25, label: 'Traffic quota', valueText: '25.00 GB / 100.00 GB' },
};

export const NearlyUsedUp: Story = {
  args: { percent: 92.4, label: 'Traffic quota', valueText: '92.40 GB / 100.00 GB' },
};

export const OverQuota: Story = {
  args: { percent: 140, label: 'Traffic quota', valueText: '140.00 GB / 100.00 GB' },
};

export const Unlimited: Story = {
  args: { percent: null, label: 'Traffic quota', valueText: '3.00 GB / Unlimited' },
};
