import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Segmented, theme } from 'antd';

import { Sparkline } from '@/components/viz';
import type { AiUsageDay } from '@/generated/zod';
import { AI_TOOL_NAME, formatTokens, formatUsd, type AiTool } from './aiUsageFormat';

type Metric = 'cost' | 'tokens';

interface AiDailyCardProps {
  tool: AiTool;
  daily: AiUsageDay[];
  isMobile: boolean;
}

/** One tool's last 30 days, as cost or as tokens; the panel's own daily-traffic card is the model. */
export default function AiDailyCard({ tool, daily, isMobile }: AiDailyCardProps) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const [metric, setMetric] = useState<Metric>('cost');
  const color = token.colorPrimary;
  const format = metric === 'cost' ? formatUsd : formatTokens;

  const series = useMemo(() => {
    const values = daily.map((d) => {
      if (metric === 'cost') return tool === 'claude' ? d.claudeCostUsd : d.codexCostUsd;
      return tool === 'claude' ? d.claudeTokens : d.codexTokens;
    });
    const sum = values.reduce((a, b) => a + b, 0);
    return {
      values,
      labels: daily.map((d) => d.day.slice(5)),
      sum,
      today: values.length ? values[values.length - 1] : 0,
      average: values.length ? sum / values.length : 0,
    };
  }, [daily, metric, tool]);

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
            <span className="ai-legend-dot" style={{ background: color }} />
            {AI_TOOL_NAME[tool]}
            <span className="ov-legend-num">{format(series.sum)}</span>
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
          data={series.values}
          labels={series.labels}
          height={isMobile ? 140 : 186}
          strokeWidth={1.75}
          fillOpacity={0.24}
          showTooltip
          showLegend={false}
          valueMax={null}
          stroke={color}
          name1={AI_TOOL_NAME[tool]}
          yFormatter={format}
        />
      </div>

      <div className="ov-wide-foot">
        <div>
          <div className="ov-kicker">{t('pages.index.traffic.periodTotal')}</div>
          <div className="ov-foot-value">{format(series.sum)}</div>
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
