import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Segmented, Table, Tag, Tooltip } from 'antd';
import { HistoryOutlined } from '@ant-design/icons';

import type { AiUsageEstimates, AiUsagePastWindow } from '@/generated/zod';
import { formatTokens, formatUsd, formatWindowSpan } from './aiUsageFormat';

type Kind = 'fiveHour' | 'weekly';

function median(values: number[]): number | null {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid];
}

/**
 * A tool's past 5-hour and weekly windows: what each used, the highest share the plan
 * reported in it and the limit that implies. A window without a reading is set against
 * the median limit of the ones with one.
 */
export default function AiWindowHistoryCard({ estimates }: { estimates: AiUsageEstimates }) {
  const { t, i18n } = useTranslation();
  const lists: Record<Kind, AiUsagePastWindow[]> = {
    fiveHour: estimates.fiveHourHistory,
    weekly: estimates.weeklyHistory,
  };
  const kinds = (['fiveHour', 'weekly'] as const).filter((k) => lists[k].length > 0);
  const [picked, setPicked] = useState<Kind>('fiveHour');
  const kind = kinds.includes(picked) ? picked : kinds[0];
  const rows = kind ? lists[kind] : [];
  const typical = median(rows.flatMap((w) => (w.limit ? [w.limit.costUsd] : [])));
  if (!kind) return null;

  return (
    <Card styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            <HistoryOutlined />
            {t('pages.aiUsage.history.title')}
          </div>
          <div className="ov-sub">{t('pages.aiUsage.history.footnote')}</div>
        </div>
        {kinds.length > 1 && (
          <Segmented<Kind>
            size="small"
            value={kind}
            onChange={setPicked}
            options={kinds.map((value) => ({
              label: t(`pages.aiUsage.history.${value}`),
              value,
            }))}
          />
        )}
      </div>
      <div className="ov-hosts-table">
        <Table<AiUsagePastWindow>
          size="small"
          rowKey={(w) => `${w.start}-${w.end}`}
          pagination={{ pageSize: 10, hideOnSinglePage: true, size: 'small' }}
          scroll={{ x: 'max-content' }}
          rowClassName={(w) => (w.current ? 'ai-window-current' : '')}
          dataSource={rows}
          columns={[
            {
              title: t('pages.aiUsage.history.window'),
              key: 'window',
              render: (_, w) => (
                <span className="ai-session-title">
                  {formatWindowSpan(w.start, w.end, i18n.language)}
                  {!w.exact && (
                    <Tooltip title={t('pages.aiUsage.history.inferredHint')}>
                      <span className="ai-window-inferred">*</span>
                    </Tooltip>
                  )}
                  {w.current && <Tag className="ai-app-tag">{t('pages.aiUsage.history.now')}</Tag>}
                </span>
              ),
            },
            {
              title: t('pages.aiUsage.history.requests'),
              key: 'requests',
              align: 'right' as const,
              render: (_, w) => w.used.requests.toLocaleString(),
            },
            {
              title: t('pages.aiUsage.history.tokens'),
              key: 'tokens',
              align: 'right' as const,
              render: (_, w) => formatTokens(w.used.totalTokens),
            },
            {
              title: t('pages.aiUsage.history.cost'),
              key: 'cost',
              align: 'right' as const,
              render: (_, w) => formatUsd(w.used.costUsd),
            },
            {
              title: t('pages.aiUsage.history.share'),
              key: 'share',
              align: 'right' as const,
              render: (_, w) => {
                if (w.peakUtilization != null) return `${Math.round(w.peakUtilization)}%`;
                if (!typical) return '—';
                return (
                  <Tooltip title={t('pages.aiUsage.history.shareEstimated')}>
                    <span className="ai-window-estimated">
                      ≈ {Math.round((w.used.costUsd / typical) * 100)}%
                    </span>
                  </Tooltip>
                );
              },
            },
            {
              title: t('pages.aiUsage.history.limit'),
              key: 'limit',
              align: 'right' as const,
              render: (_, w) => (w.limit ? `≈ ${formatUsd(w.limit.costUsd)}` : '—'),
            },
          ]}
        />
      </div>
    </Card>
  );
}
