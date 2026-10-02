import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router';
import {
  Button,
  Card,
  ConfigProvider,
  Descriptions,
  Layout,
  Result,
  Spin,
  Tag,
  Tooltip,
} from 'antd';
import { ArrowLeftOutlined, ThunderboltOutlined } from '@ant-design/icons';

import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';
import { useNodesQuery, type NodeRecord } from '@/api/queries/useNodesQuery';
import { InboundsWorkspace } from '@/pages/inbounds/InboundsWorkspace';

import GenerateNodeModal from './GenerateNodeModal';
import LocalPanelBar from './local-panel/LocalPanelBar';

const STATUS_COLORS: Record<string, string> = { online: 'green', offline: 'red' };

function HostSummary({ host }: { host: NodeRecord }) {
  const { t } = useTranslation();
  const status = host.status || 'unknown';
  return (
    <Card size="small" className="summary-card" style={{ marginBottom: 16 }}>
      <Descriptions size="small" column={{ xs: 1, sm: 2, md: 4 }}>
        <Descriptions.Item label={t('pages.nodes.status')}>
          <Tag color={STATUS_COLORS[status]}>
            {t(
              `pages.nodes.statusValues.${status === 'online' || status === 'offline' ? status : 'unknown'}`,
            )}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label={t('pages.nodes.kind')}>
          {host.kind === 'agent' ? t('pages.nodes.kindAgent') : t('pages.nodes.kindPanel')}
        </Descriptions.Item>
        <Descriptions.Item label={t('pages.nodes.address')}>
          {host.address || '-'}
        </Descriptions.Item>
        <Descriptions.Item label={t('pages.nodes.xrayVersion')}>
          {host.xrayVersion || '-'}
        </Descriptions.Item>
      </Descriptions>
    </Card>
  );
}

/** One host and the nodes it runs; `local` is this panel's own Xray. */
export default function HostPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { hostId = '' } = useParams();
  const { nodes, fetched } = useNodesQuery();
  const [generateOpen, setGenerateOpen] = useState(false);
  const [generateSession, setGenerateSession] = useState(0);

  const local = hostId === 'local';
  const host = local ? null : (nodes.find((n) => String(n.id) === hostId) ?? null);
  const backToHosts = (
    <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/nodes')}>
      {t('pages.nodes.host.allHosts')}
    </Button>
  );

  let body;
  if (!local && !fetched) {
    body = <Spin size="large" description={t('loading')} />;
  } else if (!local && !host) {
    body = <Result status="404" title={t('pages.nodes.host.notFound')} extra={backToHosts} />;
  } else {
    // Only the local panel and a connected agent can report that they run a new node.
    const canGenerate = local || host?.kind === 'agent';
    const online = local || host?.status === 'online';
    const generate = canGenerate && (
      <Tooltip title={online ? undefined : t('pages.nodes.generate.hostOffline')}>
        <Button
          icon={<ThunderboltOutlined />}
          disabled={!online}
          onClick={() => {
            setGenerateSession((n) => n + 1);
            setGenerateOpen(true);
          }}
        >
          {t('pages.nodes.generate.button')}
        </Button>
      </Tooltip>
    );
    body = (
      <>
        <PageHeader
          title={host ? host.name : t('pages.inbounds.localPanel')}
          description={host?.remark}
          extra={backToHosts}
        />
        {host ? <HostSummary host={host} /> : <LocalPanelBar />}
        <InboundsWorkspace hostScope={host ? host.id : 0} toolbarExtra={generate} />
        {generateOpen && (
          <GenerateNodeModal
            key={generateSession}
            open={generateOpen}
            host={
              host
                ? { id: host.id, name: host.name ?? '', remark: host.remark }
                : { id: 0, name: t('pages.inbounds.localPanel') }
            }
            onClose={() => setGenerateOpen(false)}
          />
        )}
      </>
    );
  }

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={`inbounds-page${isDark ? ' is-dark' : ''}${isUltra ? ' is-ultra' : ''}`}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            {body}
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
