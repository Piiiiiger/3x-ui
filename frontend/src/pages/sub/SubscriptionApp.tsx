import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Segmented } from 'antd';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';

import SubPage from './SubPage';
import PortalProbe from './portal/PortalProbe';

export default function SubscriptionApp({ data }: { data: SubPageData }) {
  const { t } = useTranslation();
  const [view, setView] = useState<'overview' | 'probe'>('overview');
  useDocumentTitle(
    t(
      view === 'probe' && data.probeBase
        ? 'subscription.portal.viewProbe'
        : 'subscription.tabLinks',
    ),
    [...new Set(data.emails?.filter(Boolean))].join(', ') || undefined,
  );
  return (
    <SubPage
      data={data}
      nav={
        data.probeBase ? (
          <Segmented
            className="portal-views"
            value={view}
            options={[
              { value: 'overview', label: t('subscription.portal.viewOverview') },
              { value: 'probe', label: t('subscription.portal.viewProbe') },
            ]}
            onChange={setView}
          />
        ) : undefined
      }
      body={
        data.probeBase && view === 'probe' ? (
          <PortalProbe
            base={data.probeBase}
            email={data.sId ?? ''}
            onSessionEnded={() => setView('overview')}
          />
        ) : undefined
      }
    />
  );
}
