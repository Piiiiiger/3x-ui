import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Button, Card, Space, Tag, Tooltip } from 'antd';
import { EditOutlined } from '@ant-design/icons';

import { TrafficMultiplierTag } from './TrafficMultiplierTag';

export interface HostNode {
  id: number;
  remark?: string;
  /** How this inbound participates in a configured proxy chain. */
  chain?: {
    role: 'target' | 'relay';
    peerNames: string[];
    enabled: boolean;
  };
}

/** Groups nodes by the host they run on; one without a host runs on the local panel (0). */
export function nodesByHostOf(
  nodes: (HostNode & { nodeId?: number | null })[],
): Map<number, HostNode[]> {
  const byHost = new Map<number, HostNode[]>();
  for (const node of nodes) {
    const host = node.nodeId ?? 0;
    byHost.set(host, [...(byHost.get(host) ?? []), node]);
  }
  return byHost;
}

// Enough names to recognise a host by, without a row of a host with many nodes wrapping.
const NAMED_NODES = 4;

/** A host's nodes as chips: the first few by name, the rest as a count. */
export function HostNodeChips({ nodes }: { nodes: HostNode[] }) {
  if (nodes.length === 0) return <span className="host-nodes-empty">-</span>;
  return (
    <span className="host-nodes">
      {nodes.slice(0, NAMED_NODES).map((node) => (
        <Tag
          key={node.id}
          color={node.chain ? (node.chain.enabled ? 'purple' : 'default') : undefined}
          title={
            node.chain
              ? node.chain.role === 'target'
                ? `已中转：经 ${node.chain.peerNames.join('、') || '未命名中转节点'}`
                : `中转入口：服务 ${node.chain.peerNames.join('、') || '未命名目标节点'}`
              : '直连节点，未配置中转'
          }
        >
          {node.remark || `#${node.id}`}
          {node.chain && (
            <small style={{ marginInlineStart: 4, opacity: 0.82 }}>
              {node.chain.enabled ? (node.chain.role === 'target' ? '中转' : '入口') : '已停用'}
            </small>
          )}
          {!node.chain && <small style={{ marginInlineStart: 4, opacity: 0.62 }}>直连</small>}
        </Tag>
      ))}
      {nodes.length > NAMED_NODES && <Tag>+{nodes.length - NAMED_NODES}</Tag>}
    </span>
  );
}

/** This panel's own Xray as a host, leading to its page like every other host. */
export function LocalPanelCard({
  nodes,
  trafficMultiplier = 1,
  onEdit,
}: {
  nodes: HostNode[];
  trafficMultiplier?: number;
  onEdit?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Card size="small" className="local-panel-card">
      <Space wrap>
        <Link to="/nodes/local">
          <strong>{t('pages.inbounds.localPanel')}</strong>
        </Link>
        <TrafficMultiplierTag multiplier={trafficMultiplier} />
        <HostNodeChips nodes={nodes} />
        {onEdit && (
          <Tooltip title={t('edit')}>
            <Button size="small" icon={<EditOutlined />} aria-label={t('edit')} onClick={onEdit} />
          </Tooltip>
        )}
      </Space>
    </Card>
  );
}
