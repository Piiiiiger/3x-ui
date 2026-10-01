import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Button, Card, theme } from 'antd';
import { ArrowRightOutlined } from '@ant-design/icons';

import type { TrafficOverview } from '@/generated/zod';

export default function ClientCountsCard({ overview }: { overview: TrafficOverview }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token } = theme.useToken();

  // The five states partition the clients, so the bar adds up to the total.
  const states = [
    {
      key: 'normal',
      label: t('pages.index.traffic.normal'),
      count: overview.active - overview.expiring,
      color: token.colorSuccess,
    },
    {
      key: 'expiring',
      label: t('pages.index.traffic.expiring'),
      count: overview.expiring,
      color: token.colorWarning,
    },
    {
      key: 'usedUp',
      label: t('pages.index.traffic.usedUp'),
      count: overview.usedUp,
      color: token.colorError,
    },
    {
      key: 'expired',
      label: t('pages.index.traffic.expired'),
      count: overview.expired,
      color: token.colorErrorActive,
    },
    {
      key: 'disabled',
      label: t('pages.index.traffic.disabled'),
      count: overview.disabled,
      color: token.colorTextQuaternary,
    },
  ];

  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div>
          <div className="ov-kicker ov-card-title">{t('pages.plans.people')}</div>
          <div className="ov-sub">
            {t('pages.index.traffic.clientsSub', { count: overview.clients })}
          </div>
        </div>
      </div>
      <div className="ov-count-bar" aria-hidden="true">
        {states
          .filter((s) => s.count > 0)
          .map((s) => (
            <span key={s.key} style={{ flexGrow: s.count, background: s.color }} />
          ))}
      </div>
      <ul className="ov-counts">
        {states.map((s) => (
          <li key={s.key}>
            <span className="ov-count-dot" style={{ background: s.color }} />
            <span>{s.label}</span>
            <span className="ov-count-num">{s.count}</span>
          </li>
        ))}
      </ul>
      <div className="ov-counts-foot">
        <Button type="link" size="small" onClick={() => navigate('/clients')}>
          {t('pages.index.traffic.manageClients')}
          <ArrowRightOutlined />
        </Button>
      </div>
    </Card>
  );
}
