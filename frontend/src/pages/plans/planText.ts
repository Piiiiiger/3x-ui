import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { formatInboundLabel } from '@/lib/inbounds/label';

export interface InboundChoice {
  label: string;
  value: number;
}

// Inbounds grouped by the panel that runs them: this one first, then each node.
export function useInboundChoices() {
  const { t } = useTranslation();
  const { data: inbounds } = useInboundOptions();
  const { nodes } = useNodesQuery();
  return useMemo(() => {
    const nodeName = new Map(nodes.map((n) => [n.id, n.name || n.remark || `#${n.id}`]));
    const groups = new Map<string, InboundChoice[]>();
    const byId = new Map<number, string>();
    for (const ib of inbounds ?? []) {
      const panel =
        ib.nodeId != null
          ? (nodeName.get(ib.nodeId) ?? `#${ib.nodeId}`)
          : t('pages.inbounds.localPanel');
      const label = formatInboundLabel(ib.tag, ib.remark);
      byId.set(ib.id, label);
      groups.set(panel, [...(groups.get(panel) ?? []), { label, value: ib.id }]);
    }
    const options = [...groups.entries()].map(([panel, items]) => ({
      label: panel,
      title: panel,
      options: items,
    }));
    const flat = options.flatMap((g) => g.options);
    return { options, flat, labelOf: (id: number) => byId.get(id) ?? `#${id}` };
  }, [inbounds, nodes, t]);
}

// The terms a plan can be sold by, and how people name them.
export const PLAN_TERMS = [30, 90, 180, 365];
const TERM_NAMES: Record<number, string> = { 30: '月付', 90: '季付', 180: '半年付', 365: '年付' };

export function termLabel(days: number): string {
  const name = TERM_NAMES[days];
  return name ? `${name}（${days} 天）` : `${days} 天`;
}

export function termsText(days: number[] | undefined): string {
  if (!days?.length) return '不限';
  return days.map((d) => TERM_NAMES[d] ?? `${d} 天`).join(' · ');
}
