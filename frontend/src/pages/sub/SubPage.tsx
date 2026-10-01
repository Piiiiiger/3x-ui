import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Tabs, message } from 'antd';
import type { TabsProps } from 'antd';
import {
  AppstoreOutlined,
  ClockCircleOutlined,
  CustomerServiceOutlined,
  LinkOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';

import { ClipboardManager } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import SubAppsTab from './SubAppsTab';
import SubConfigsTab from './SubConfigsTab';
import SubHeader from './SubHeader';
import SubHero from './SubHero';
import SubLinksTab from './SubLinksTab';
import SubShell, { useSubLanguage } from './SubShell';
import { buildSubApps, daysUntil, detectPlatform, resolveSubStatus } from './subPageModel';
import './SubPage.css';

// buildSubView derives what the page shows from the server's page data.
function buildSubView(subData: SubPageData, loadedAt: number) {
  const sId = subData.sId || '';
  const subUrl = subData.subUrl || '';
  const subTitle = subData.subTitle || '';
  const linkEmails: string[] = Array.isArray(subData.emails) ? subData.emails : [];
  const totalByte = Number(subData.totalByte || 0);
  const usedByte =
    Number(subData.usedByte || 0) ||
    Number(subData.downloadByte || 0) + Number(subData.uploadByte || 0);
  const expireMs = Number(subData.expire || 0) * 1000;
  return {
    sId,
    subUrl,
    subJsonUrl: subData.subJsonUrl || '',
    subClashUrl: subData.subClashUrl || '',
    subTitle,
    subSupportUrl: subData.subSupportUrl || '',
    updateHours: Number(subData.subUpdates || 0),
    announce: subData.announce || '',
    links: Array.isArray(subData.links) ? subData.links : [],
    clientEmail: [...new Set(linkEmails.filter(Boolean))].join(', '),
    heroData: {
      status: resolveSubStatus(
        { enabled: !!subData.enabled, usedByte, totalByte, expireMs },
        loadedAt,
      ),
      daysLeft: daysUntil(expireMs, loadedAt),
      usedByte,
      totalByte,
      expireMs,
      lastOnlineMs: Number(subData.lastOnline || 0),
      download: subData.download || '0',
      upload: subData.upload || '0',
      used: subData.used || '0',
      total: subData.total || '∞',
      remained: subData.remained || '',
      datepicker: subData.datepicker || 'gregorian',
    },
    apps: buildSubApps({ subUrl, sId, subTitle }),
  };
}

interface SubPageProps {
  data: SubPageData;
  // The portal adds its sign-out button to the toolbar and its cards below the usage.
  headerExtra?: ReactNode;
  children?: ReactNode;
}

export default function SubPage({ data, headerExtra, children }: SubPageProps) {
  const { t } = useTranslation();
  const [loadedAt] = useState(() => Date.now());
  const [initialPlatform] = useState(() => detectPlatform(navigator.userAgent));
  const view = useMemo(() => buildSubView(data, loadedAt), [data, loadedAt]);
  const { subUrl, subJsonUrl, subClashUrl, links, apps } = view;
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);
  const { lang, onLangChange } = useSubLanguage();

  const copy = useCallback(
    async (value: string, toast?: string) => {
      if (!value) return;
      const ok = await ClipboardManager.copyText(value);
      if (ok) messageApi.success(toast ?? t('copied'));
    },
    [t, messageApi],
  );

  const open = useCallback((url: string) => {
    if (url) window.open(url, '_blank');
  }, []);

  const tabs = useMemo(() => {
    const items: NonNullable<TabsProps['items']> = [];
    if (subUrl || subJsonUrl || subClashUrl) {
      items.push({
        key: 'subscription',
        icon: <LinkOutlined />,
        label: t('subscription.tabLinks'),
        children: (
          <SubLinksTab
            subUrl={subUrl}
            subJsonUrl={subJsonUrl}
            subClashUrl={subClashUrl}
            onCopy={copy}
          />
        ),
      });
    }
    if (subUrl) {
      items.push({
        key: 'apps',
        icon: <AppstoreOutlined />,
        label: t('subscription.tabApps'),
        children: <SubAppsTab apps={apps} initialPlatform={initialPlatform} onOpen={open} />,
      });
    }
    if (links.length > 0) {
      items.push({
        key: 'configs',
        icon: <UnorderedListOutlined />,
        label: (
          <>
            {t('subscription.tabConfigs')}
            <span className="sub-tab-count">{links.length}</span>
          </>
        ),
        children: <SubConfigsTab links={links} onCopy={copy} />,
      });
    }
    return items;
  }, [t, copy, open, subUrl, subJsonUrl, subClashUrl, links, apps, initialPlatform]);

  return (
    <SubShell lang={lang}>
      {messageContextHolder}
      <SubHeader
        title={view.subTitle}
        sId={view.sId}
        email={view.clientEmail}
        lang={lang}
        onLangChange={onLangChange}
        extra={headerExtra}
      />
      {view.announce && (
        <Alert type="info" showIcon title={view.announce} className="sub-announce" />
      )}
      <SubHero {...view.heroData} lang={lang} />
      {children}
      {tabs.length > 0 && <Tabs className="sub-tabs" tabBarGutter={24} items={tabs} />}
      {(view.updateHours > 0 || view.subSupportUrl) && (
        <footer className="sub-footer">
          {view.updateHours > 0 && (
            <span>
              <ClockCircleOutlined />
              {t('subscription.updateInterval', { hours: view.updateHours })}
            </span>
          )}
          {view.subSupportUrl && (
            <a href={view.subSupportUrl} target="_blank" rel="noopener noreferrer">
              <CustomerServiceOutlined />
              {t('subscription.support')}
            </a>
          )}
        </footer>
      )}
    </SubShell>
  );
}
