import type { Meta, StoryObj } from '@storybook/react-vite';
import { Button } from 'antd';
import { PlusOutlined } from '@ant-design/icons';

import PageHeader from './PageHeader';

const meta = {
  title: 'UI/PageHeader',
  component: PageHeader,
  tags: ['autodocs'],
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component:
          'Page title block shown at the top of every management page: a large title, an optional one-line description, and optional actions aligned to the right.',
      },
    },
  },
  argTypes: {
    title: { description: 'Page name, rendered as the page `h1`.' },
    description: { description: 'One sentence under the title saying what the page manages.' },
    extra: { description: 'Actions aligned to the right of the title (buttons, links).' },
  },
} satisfies Meta<typeof PageHeader>;

export default meta;

type Story = StoryObj<typeof meta>;

export const TitleAndDescription: Story = {
  args: {
    title: 'Inbounds',
    description: 'Listening ports, protocols and transports, with the clients each one serves.',
  },
};

export const WithActions: Story = {
  args: {
    title: 'Hosts',
    description: 'Addresses and TLS details that share links and subscriptions hand out.',
    extra: (
      <Button type="primary" icon={<PlusOutlined />}>
        Add Host
      </Button>
    ),
  },
};

export const TitleOnly: Story = {
  args: { title: 'Panel Settings' },
};
