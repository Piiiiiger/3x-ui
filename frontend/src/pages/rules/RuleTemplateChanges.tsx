import { useTranslation } from 'react-i18next';
import { Tag } from 'antd';

import type { RuleTemplateChange } from '@/generated/zod';

// The keys named in words; any other shows as it is written in the YAML.
const CHANGE_LABELS: Record<string, string> = {
  rules: 'pages.rules.changeKeys.rules',
  'proxy-groups': 'pages.rules.changeKeys.groups',
  dns: 'pages.rules.changeKeys.dns',
  listeners: 'pages.rules.changeKeys.listeners',
};

/** What a variant changes in its base, e.g. 代理组 · DNS · 规则 +5. */
export default function RuleTemplateChanges({ changes }: { changes: RuleTemplateChange[] }) {
  const { t } = useTranslation();
  if (changes.length === 0) return <Tag>{t('pages.rules.noChanges')}</Tag>;
  return (
    <span className="rule-template-changes">
      {changes.map((change) => {
        const label = CHANGE_LABELS[change.key] ? t(CHANGE_LABELS[change.key]) : change.key;
        return (
          <Tag key={change.key} color="cyan">
            {change.added > 0 ? `${label} +${change.added}` : label}
          </Tag>
        );
      })}
    </span>
  );
}
