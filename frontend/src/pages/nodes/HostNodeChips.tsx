import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Card, Space, Tag } from 'antd';

export interface HostNode {
  id: number;
  remark?: string;
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
        <Tag key={node.id}>{node.remark || `#${node.id}`}</Tag>
      ))}
      {nodes.length > NAMED_NODES && <Tag>+{nodes.length - NAMED_NODES}</Tag>}
    </span>
  );
}

/** This panel's own Xray as a host, leading to its page like every other host. */
export function LocalPanelCard({ nodes }: { nodes: HostNode[] }) {
  const { t } = useTranslation();
  return (
    <Card size="small" className="local-panel-card">
      <Space wrap>
        <Link to="/nodes/local">
          <strong>{t('pages.inbounds.localPanel')}</strong>
        </Link>
        <HostNodeChips nodes={nodes} />
      </Space>
    </Card>
  );
}
