import type { Meta, StoryObj } from '@storybook/react-vite';
import { ConfigProvider } from 'antd';

import { buildAntdThemeConfig } from '@/hooks/useTheme';
import { subPageTheme } from '../SubShell';
import PortalRules from './PortalRules';

const meta = {
  title: 'Portal/PortalRules',
  component: PortalRules,
  tags: ['autodocs'],
  decorators: [
    (Story, context) => {
      const dark = context.globals.theme === 'dark';
      const theme = subPageTheme(buildAntdThemeConfig(dark, false), dark);
      // Without motion the dialog is opaque at once, so axe can measure its contrast.
      return (
        <ConfigProvider theme={{ ...theme, token: { ...theme.token, motion: false } }}>
          <Story />
        </ConfigProvider>
      );
    },
  ],
  beforeEach: () => {
    localStorage.removeItem('pigger.portal.rules.snoozedUntil');
  },
  parameters: {
    docs: {
      description: {
        component:
          "The portal's 使用须知: it opens on each visit until snoozed for 7 days. The phone location warning comes first, then the rules the system enforces, their penalties, and how AI command-line tools reach the AI group.",
      },
    },
  },
  argTypes: {
    limitIp: {
      description: "The plan's online IP limit; 0 or missing says only that there is one.",
    },
  },
  args: { limitIp: 3 },
} satisfies Meta<typeof PortalRules>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Light: Story = {};

export const Dark: Story = { globals: { theme: 'dark' } };

export const WithoutAnIpLimit: Story = { args: { limitIp: 0 } };
