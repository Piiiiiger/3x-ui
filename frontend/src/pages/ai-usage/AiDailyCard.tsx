import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Segmented, theme } from 'antd';

import { Sparkline } from '@/components/viz';
import type { AiUsageDay } from '@/generated/zod';
import { formatTokens, formatUsd } from './aiUsageFormat';

type Metric = 'cost' | 'tokens';

interface AiDailyCardProps {
  daily: AiUsageDay[];
  isMobile: boolean;
}

/** The last 30 days per app, as cost or as tokens; the panel's own daily-traffic card is the model. */
export default function AiDailyCard({ daily, isMobile }: AiDailyCardProps) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const [metric, setMetric] = useState<Metric>('cost');
  const claudeColor = token.colorPrimary;
  const codexColor = token.colorTextTertiary;
  const format = metric === 'cost' ? formatUsd : formatTokens;

  const series = useMemo(() => {
    const claude = daily.map((d) => (metric === 'cost' ? d.claudeCostUsd : d.claudeTokens));
    const codex = daily.map((d) => (metric === 'cost' ? d.codexCostUsd : d.codexTokens));
    const claudeSum = claude.reduce((a, b) => a + b, 0);
    const codexSum = codex.reduce((a, b) => a + b, 0);
    const last = daily.length - 1;
    return {
      claude,
      codex,
      labels: daily.map((d) => d.day.slice(5)),
      claudeSum,
      codexSum,
      today: last >= 0 ? claude[last] + codex[last] : 0,
      average: daily.length ? (claudeSum + codexSum) / daily.length : 0,
    };
  }, [daily, metric]);

  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div>
          <div className="ov-kicker ov-card-title">
            {metric === 'cost' ? t('pages.aiUsage.dailyCost') : t('pages.aiUsage.dailyTokens')}
          </div>
          <div className="ov-sub">{t('pages.aiUsage.dailySub', { days: daily.length })}</div>
        </div>
        <div className="ov-wide-legend ai-daily-legend">
          <div className="ov-legend-label">
            <span className="ai-legend-dot" style={{ background: claudeColor }} />
            Claude Code
            <span className="ov-legend-num">{format(series.claudeSum)}</span>
          </div>
          <div className="ov-legend-label">
            <span className="ai-legend-dot" style={{ background: codexColor }} />
            Codex
            <span className="ov-legend-num">{format(series.codexSum)}</span>
          </div>
          <Segmented<Metric>
            size="small"
            value={metric}
            onChange={setMetric}
            options={[
              { label: t('pages.aiUsage.metricCost'), value: 'cost' },
              { label: t('pages.aiUsage.metricTokens'), value: 'tokens' },
            ]}
          />
        </div>
      </div>

      <div className="ov-wide-chart">
        <Sparkline
          data={series.claude}
          data2={series.codex}
          labels={series.labels}
          height={isMobile ? 140 : 186}
          strokeWidth={1.75}
          fillOpacity={0.24}
          showTooltip
          showLegend={false}
          valueMax={null}
          stroke={claudeColor}
          stroke2={codexColor}
          name1="Claude Code"
          name2="Codex"
          yFormatter={format}
        />
      </div>

      <div className="ov-wide-foot">
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.periodTotal')}</div>
          <div className="ov-foot-value">{format(series.claudeSum + series.codexSum)}</div>
        </div>
        <span className="ov-foot-sep" />
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.today')}</div>
          <div className="ov-foot-value">{format(series.today)}</div>
        </div>
        <span className="ov-foot-sep" />
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.dailyAverage')}</div>
          <div className="ov-foot-value">{format(series.average)}</div>
        </div>
      </div>
    </Card>
  );
}
