import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Tooltip } from 'antd';
import { ProfileOutlined } from '@ant-design/icons';

import { RainbowBar } from '@/components/ui';
import { SizeFormatter } from '@/utils';

interface PlanUsageCellProps {
  email: string;
  planName?: string;
  used: number;
  total: number;
  onPlanClick: (email: string) => void;
}

/** 套餐/流量: the plan's name over a bar of the quota used, as on 妙妙屋X. */
const PlanUsageCell = memo(function PlanUsageCell({
  email,
  planName,
  used,
  total,
  onPlanClick,
}: PlanUsageCellProps) {
  const { t } = useTranslation();
  const percent = total > 0 ? Math.min(100, Math.floor((used / total) * 100)) : 0;
  const amount = `${SizeFormatter.sizeFormat(used)} / ${total > 0 ? SizeFormatter.sizeFormat(total) : '∞'}`;
  return (
    <div className="plan-usage">
      <div className="plan-usage-head">
        {planName ? (
          <Button
            type="link"
            size="small"
            className="plan-usage-name"
            onClick={() => onPlanClick(email)}
          >
            {planName}
          </Button>
        ) : (
          <Button size="small" icon={<ProfileOutlined />} onClick={() => onPlanClick(email)}>
            {t('pages.plans.assign')}
          </Button>
        )}
        <Tooltip title={amount}>
          <span className="plan-usage-value">
            {total > 0 ? `${percent}%` : SizeFormatter.sizeFormat(used)}
          </span>
        </Tooltip>
      </div>
      <RainbowBar
        percent={total > 0 ? (used / total) * 100 : null}
        label={t('usage')}
        valueText={amount}
      />
    </div>
  );
});

export default PlanUsageCell;
