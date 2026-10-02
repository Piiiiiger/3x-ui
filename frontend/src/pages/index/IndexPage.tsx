import { useEffect } from 'react';
import { ConfigProvider, Layout, message } from 'antd';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppNav from '@/layouts/AppNav';
import { setMessageInstance } from '@/utils/messageBus';
import TrafficOverviewSection from './TrafficOverviewSection';
import './IndexPage.css';

/** 流量信息: the servers' quotas, the daily chart, and who used what in the period. */
export default function IndexPage() {
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const pageClass = `index-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="ov-page">
              <TrafficOverviewSection isMobile={isMobile} />
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
