import type { Meta, StoryObj } from '@storybook/react-vite';

import BrandMark from './BrandMark';

const meta = {
  title: 'UI/BrandMark',
  component: BrandMark,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component:
          "The panel's name with its first letter drawn the way 妙妙屋X draws its X: a flowing warm gradient, a soft glow and two sparkles. The name stays one word to screen readers; the glow and sparkles are hidden from them.",
      },
    },
  },
  argTypes: {
    className: { description: 'Extra CSS class for the wrapper, e.g. to size the mark.' },
  },
} satisfies Meta<typeof BrandMark>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <span style={{ fontSize: 22, fontWeight: 700 }}>
      <BrandMark />
    </span>
  ),
};

export const LoginSize: Story = {
  render: () => (
    <span style={{ fontSize: 34, fontWeight: 700 }}>
      <BrandMark />
    </span>
  ),
};
