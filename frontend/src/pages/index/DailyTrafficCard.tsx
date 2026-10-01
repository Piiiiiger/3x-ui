import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, theme } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons';

import { SizeFormatter } from '@/utils';
import { Sparkline } from '@/components/viz';
import type { TrafficDay } from '@/generated/zod';

interface DailyTrafficCardProps {
  daily: TrafficDay[];
  isMobile: boolean;
}

export default function DailyTrafficCard({ daily, isMobile }: DailyTrafficCardProps) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const accent = token.colorPrimary;
  const downColor = token.colorTextTertiary;

  const series = useMemo(() => {
    const up = daily.map((d) => d.up);
    const down = daily.map((d) => d.down);
    const upSum = up.reduce((a, b) => a + b, 0);
    const downSum = down.reduce((a, b) => a + b, 0);
    const last = daily.at(-1);
    return {
      up,
      down,
      labels: daily.map((d) => d.day.slice(5)),
      upSum,
      downSum,
      today: last ? last.up + last.down : 0,
      average: daily.length ? (upSum + downSum) / daily.length : 0,
    };
  }, [daily]);

  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div>
          <div className="ov-kicker ov-card-title">{t('pages.index.traffic.daily')}</div>
          <div className="ov-sub">{t('pages.index.traffic.dailySub', { days: daily.length })}</div>
        </div>
        <div className="ov-wide-legend">
          <div className="ov-legend-label">
            <ArrowUpOutlined style={{ color: accent }} />
            {t('pages.index.upload')}
            <span className="ov-legend-num">{SizeFormatter.sizeFormat(series.upSum)}</span>
          </div>
          <div className="ov-legend-label">
            <ArrowDownOutlined style={{ color: downColor }} />
            {t('pages.index.download')}
            <span className="ov-legend-num">{SizeFormatter.sizeFormat(series.downSum)}</span>
          </div>
        </div>
      </div>

      <div className="ov-wide-chart">
        <Sparkline
          data={series.up}
          data2={series.down}
          labels={series.labels}
          height={isMobile ? 140 : 186}
          strokeWidth={1.75}
          fillOpacity={0.24}
          showTooltip
          showLegend={false}
          valueMax={null}
          stroke={accent}
          stroke2={downColor}
          name1={t('pages.index.upload')}
          name2={t('pages.index.download')}
          yFormatter={SizeFormatter.sizeFormat}
        />
      </div>

      <div className="ov-wide-foot">
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.periodTotal')}</div>
          <div className="ov-foot-value">
            {SizeFormatter.sizeFormat(series.upSum + series.downSum)}
          </div>
        </div>
        <span className="ov-foot-sep" />
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.today')}</div>
          <div className="ov-foot-value">{SizeFormatter.sizeFormat(series.today)}</div>
        </div>
        <span className="ov-foot-sep" />
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.dailyAverage')}</div>
          <div className="ov-foot-value">{SizeFormatter.sizeFormat(series.average)}</div>
        </div>
      </div>
    </Card>
  );
}
