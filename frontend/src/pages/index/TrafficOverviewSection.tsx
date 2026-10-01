import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Progress, theme } from 'antd';
import {
  CloudServerOutlined,
  HourglassOutlined,
  PieChartOutlined,
  SwapOutlined,
} from '@ant-design/icons';

import { SizeFormatter } from '@/utils';
import { usageTierColor } from '@/models/status';
import { useTrafficOverviewQuery } from '@/api/queries/useTrafficOverviewQuery';
import StatTile from './StatTile';
import DailyTrafficCard from './DailyTrafficCard';
import ClientCountsCard from './ClientCountsCard';
import AttentionCard from './AttentionCard';

function sizeParts(bytes: number): [string, string] {
  const [value, unit = ''] = SizeFormatter.sizeFormat(bytes).split(' ');
  return [value, unit];
}

export default function TrafficOverviewSection({ isMobile }: { isMobile: boolean }) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const { overview, fetched, fetchError, refetch } = useTrafficOverviewQuery();

  if (!fetched) return <Card loading />;
  if (!overview) {
    return (
      <Alert
        type="error"
        showIcon
        title={t('somethingWentWrong')}
        description={fetchError}
        action={
          <Button size="small" onClick={() => refetch()}>
            {t('refresh')}
          </Button>
        }
      />
    );
  }

  const quota = overview.quotaBytes;
  const consumed = quota - overview.remainingBytes;
  const rate = quota > 0 ? (consumed / quota) * 100 : 0;
  const today = overview.daily.at(-1);
  const [quotaValue, quotaUnit] = sizeParts(quota);
  const [usedValue, usedUnit] = sizeParts(overview.usedBytes);
  const [remainingValue, remainingUnit] = sizeParts(overview.remainingBytes);

  return (
    <>
      <div className="ov-stats">
        <StatTile
          icon={<CloudServerOutlined />}
          label={t('pages.index.traffic.quota')}
          value={quotaValue}
          unit={quotaUnit}
          detail={t('pages.index.traffic.quotaDetail', {
            limited: overview.clients - overview.unlimited,
            unlimited: overview.unlimited,
          })}
        />
        <StatTile
          icon={<SwapOutlined />}
          label={t('pages.index.traffic.used')}
          value={usedValue}
          unit={usedUnit}
          detail={t('pages.index.traffic.usedDetail', {
            size: SizeFormatter.sizeFormat(today ? today.up + today.down : 0),
          })}
        />
        <StatTile
          icon={<HourglassOutlined />}
          label={t('pages.index.traffic.remaining')}
          value={remainingValue}
          unit={remainingUnit}
          detail={t('pages.index.traffic.remainingDetail')}
        />
        <StatTile
          icon={<PieChartOutlined />}
          label={t('pages.index.traffic.usageRate')}
          value={quota > 0 ? rate.toFixed(1) : '—'}
          unit={quota > 0 ? '%' : undefined}
          detail={
            quota > 0
              ? t('pages.index.traffic.usageRateDetail', {
                  used: SizeFormatter.sizeFormat(consumed),
                  quota: SizeFormatter.sizeFormat(quota),
                })
              : t('pages.index.traffic.noQuota')
          }
        >
          {quota > 0 && (
            <Progress
              percent={rate}
              showInfo={false}
              size="small"
              strokeColor={usageTierColor(rate, token.colorPrimary)}
            />
          )}
        </StatTile>
      </div>

      <div className="ov-mid">
        <DailyTrafficCard daily={overview.daily} isMobile={isMobile} />
        <ClientCountsCard overview={overview} />
      </div>

      <AttentionCard clients={overview.attention} />
    </>
  );
}
