import type { Meta, StoryObj } from '@storybook/react-vite';

import BrandIcon from './BrandIcon';

const meta = {
  title: 'UI/BrandIcon',
  component: BrandIcon,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component: 'Taffy avatar, displayed beside the accessible Pigger wordmark.',
      },
    },
  },
  argTypes: {
    className: { description: 'Optional class for sizing the mascot in its header.' },
  },
} satisfies Meta<typeof BrandIcon>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
