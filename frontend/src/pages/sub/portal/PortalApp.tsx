import { useCallback, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Result, Segmented, Spin } from 'antd';
import { LogoutOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { PANEL_NAME } from '@/lib/brand';
import { getMessage } from '@/utils/messageBus';
import { PortalDataSchema, type PortalData } from '@/schemas/portal';
import SubHeader from '../SubHeader';
import SubPage from '../SubPage';
import SubShell, { useSubLanguage } from '../SubShell';
import { PortalPlanCard, PortalUsageCard } from './PortalCards';
import PortalLogin from './PortalLogin';
import PortalProbe from './PortalProbe';
import PortalRedeemModal from './PortalRedeemModal';
import PortalCustomize from './PortalCustomize';
import './Portal.css';

type PortalView = 'overview' | 'probe' | 'customize';

const PROBE_HASH = '#probe';
const CUSTOMIZE_HASH = '#customize';

// The view lives in the address, so that a reload keeps it; the portal has no router.
function usePortalView(): [PortalView, (next: PortalView) => void] {
  const [view, setView] = useState<PortalView>(() =>
    window.location.hash === PROBE_HASH
      ? 'probe'
      : window.location.hash === CUSTOMIZE_HASH
        ? 'customize'
        : 'overview',
  );
  const show = useCallback((next: PortalView) => {
    const { pathname, search } = window.location;
    // replaceState, because clearing location.hash leaves a bare "#" behind.
    const hash = next === 'probe' ? PROBE_HASH : next === 'customize' ? CUSTOMIZE_HASH : '';
    window.history.replaceState(null, '', pathname + search + hash);
    setView(next);
  }, []);
  return [view, show];
}

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
  const [customizationDirty, setCustomizationDirty] = useState(false);
  const [view, showView] = usePortalView();
  const [redeeming, setRedeeming] = useState(false);
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
    // The next person to sign in on this browser must not be shown these servers.
    queryClient.removeQueries({ queryKey: keys.portal.probes() });
    queryClient.setQueryData(keys.portal.data(base), null);
  }, [base, queryClient]);

  // Reset rather than invalidate: the signed-in page must leave the screen now. Left
  // mounted, its probe view would ask the dead session again and restart this very answer.
  const onSessionEnded = useCallback(() => {
    queryClient.removeQueries({ queryKey: keys.portal.probes() });
    void queryClient.resetQueries({ queryKey: keys.portal.data(base) });
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

  // The address may still name the probe view of whoever signed in before.
  const probeOpen = data.probe && view === 'probe';
  const customizeOpen = view === 'customize';
  return (
    <SubPage
      data={data.page ?? { emails: [data.email], enabled: true }}
      headerExtra={
        <>
          <Button onClick={() => setRedeeming(true)}>{t('subscription.portal.redeem')}</Button>
          <Button
            size="large"
            className="toolbar-btn"
            icon={<LogoutOutlined />}
            aria-label={t('subscription.portal.signOut')}
            title={t('subscription.portal.signOut')}
            onClick={() => {
              if (!customizationDirty || window.confirm('自定义订阅有未保存修改，确定退出？')) {
                setCustomizationDirty(false);
                void signOut();
              }
            }}
          />
        </>
      }
      nav={
        <Segmented<PortalView>
          className="portal-views"
          value={customizeOpen ? 'customize' : probeOpen ? 'probe' : 'overview'}
          options={[
            { value: 'overview', label: t('subscription.portal.viewOverview') },
            ...(data.probe
              ? [{ value: 'probe' as const, label: t('subscription.portal.viewProbe') }]
              : []),
            { value: 'customize' as const, label: '自定义订阅' },
          ]}
          onChange={(next) => {
            if (!customizationDirty || window.confirm('自定义订阅有未保存修改，确定离开？')) {
              setCustomizationDirty(false);
              showView(next);
            }
          }}
        />
      }
      body={
        customizeOpen ? (
          <PortalCustomize
            base={base}
            onSessionEnded={onSessionEnded}
            onDirtyChange={setCustomizationDirty}
          />
        ) : probeOpen ? (
          <PortalProbe base={base} email={data.email} onSessionEnded={onSessionEnded} />
        ) : undefined
      }
    >
      {!data.page && (
        <Alert type="warning" showIcon title={t('subscription.portal.noSubscription')} />
      )}
      {data.plan && <PortalPlanCard plan={data.plan} />}
      {redeeming && (
        <PortalRedeemModal
          base={base}
          onClose={() => setRedeeming(false)}
          onSessionEnded={onSessionEnded}
          onActivated={() => {
            void getMessage().success(t('subscription.portal.activated'));
            queryClient.removeQueries({ queryKey: keys.portal.probes() });
            onSignedIn();
          }}
        />
      )}
      <PortalUsageCard daily={data.daily} />
    </SubPage>
  );
}
