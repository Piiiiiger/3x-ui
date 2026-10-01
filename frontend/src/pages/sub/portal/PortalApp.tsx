import { useCallback } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Result, Spin } from 'antd';
import { LogoutOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { PANEL_NAME } from '@/lib/brand';
import { PortalDataSchema, type PortalData } from '@/schemas/portal';
import SubHeader from '../SubHeader';
import SubPage from '../SubPage';
import SubShell, { useSubLanguage } from '../SubShell';
import { PortalPlanCard, PortalUsageCard } from './PortalCards';
import PortalLogin from './PortalLogin';
import './Portal.css';

// null means nobody is signed in; any other failure is an error worth retrying.
async function fetchPortal(base: string): Promise<PortalData | null> {
  const res = await fetch(`${base}/data`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (!res.ok) {
    // Drain the body so the request completes instead of lingering open.
    await res.text().catch(() => '');
    if (res.status === 401) return null;
    throw new Error(`portal data: HTTP ${res.status}`);
  }
  return PortalDataSchema.parse(await res.json());
}

function PortalStatus({ children }: { children: ReactNode }) {
  const { lang, onLangChange } = useSubLanguage();
  return (
    <SubShell lang={lang}>
      <SubHeader title={PANEL_NAME} sId="" email="" lang={lang} onLangChange={onLangChange} />
      <div className="portal-status">{children}</div>
    </SubShell>
  );
}

export default function PortalApp({ base }: { base: string }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const portal = useQuery({
    queryKey: keys.portal.data(base),
    queryFn: () => fetchPortal(base),
    retry: false,
  });

  const onSignedIn = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: keys.portal.data(base) });
  }, [base, queryClient]);

  const signOut = useCallback(async () => {
    await fetch(`${base}/logout`, { method: 'POST', credentials: 'same-origin' }).catch(() => null);
    queryClient.setQueryData(keys.portal.data(base), null);
  }, [base, queryClient]);

  if (portal.isPending) {
    return (
      <PortalStatus>
        <Spin size="large" />
      </PortalStatus>
    );
  }
  if (portal.isError) {
    return (
      <PortalStatus>
        <Result
          status="warning"
          title={t('subscription.portal.loadFailed')}
          extra={
            <Button type="primary" onClick={() => portal.refetch()}>
              {t('refresh')}
            </Button>
          }
        />
      </PortalStatus>
    );
  }
  const data = portal.data;
  if (!data) return <PortalLogin base={base} onSignedIn={onSignedIn} />;

  return (
    <SubPage
      data={data.page ?? { emails: [data.email], enabled: true }}
      headerExtra={
        <Button
          size="large"
          className="toolbar-btn"
          icon={<LogoutOutlined />}
          aria-label={t('subscription.portal.signOut')}
          title={t('subscription.portal.signOut')}
          onClick={signOut}
        />
      }
    >
      {!data.page && (
        <Alert type="warning" showIcon title={t('subscription.portal.noSubscription')} />
      )}
      {data.plan && <PortalPlanCard plan={data.plan} />}
      <PortalUsageCard daily={data.daily} />
    </SubPage>
  );
}
