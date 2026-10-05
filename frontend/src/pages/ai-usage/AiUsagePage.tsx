import { useEffect } from 'react';
import { ConfigProvider, Layout, message } from 'antd';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppNav from '@/layouts/AppNav';
import { setMessageInstance } from '@/utils/messageBus';
import AiUsageSection from './AiUsageSection';
import '@/pages/index/IndexPage.css';
import './AiUsagePage.css';

/** AI 用量 shares the traffic page's cards and spacing, so it carries that page's class too. */
export default function AiUsagePage() {
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const pageClass =
    `index-page ai-usage-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="ov-page">
              <AiUsageSection isMobile={isMobile} />
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
