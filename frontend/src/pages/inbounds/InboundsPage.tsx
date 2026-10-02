import { useTranslation } from 'react-i18next';
import { ConfigProvider, Layout } from 'antd';

import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';

import { InboundsWorkspace } from './InboundsWorkspace';

export default function InboundsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={`inbounds-page${isDark ? ' is-dark' : ''}${isUltra ? ' is-ultra' : ''}`}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader title={t('menu.inbounds')} description={t('pages.inbounds.intro')} />
            <InboundsWorkspace />
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
