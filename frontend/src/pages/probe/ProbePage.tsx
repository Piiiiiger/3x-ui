import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Layout,
  Result,
  Row,
  Space,
  Spin,
  Statistic,
  Tag,
  message,
} from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  ExportOutlined,
  LinkOutlined,
  RadarChartOutlined,
  SettingOutlined,
} from '@ant-design/icons';

import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import ProbeServerCard, { ProbeMeter } from '@/components/probe/ProbeServerCard';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useProbeQuery } from '@/api/queries/useProbeQuery';
import type { ProbeServer } from '@/generated/zod';
import { SizeFormatter, TimeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import ProbeLinksModal from './ProbeLinksModal';
import ProbeSettingsModal from './ProbeSettingsModal';
import { probeHostLabel } from './probeHostLabel';
import './ProbePage.css';

// A server that never reported has none of these, and "none" is what a
// bare-metal one reports as its virtualization.
function systemLine(server: ProbeServer, t: TFunction): string {
  const virtualization = server.virtualization === 'none' ? '' : server.virtualization;
  const cores = server.cpuCores > 0 ? t('pages.probe.cores', { count: server.cpuCores }) : '';
  return [server.os, server.arch, virtualization, cores].filter(Boolean).join(' · ');
}

function ServerFooter({ server }: { server: ProbeServer }) {
  const { t } = useTranslation();
  const limited = server.trafficLimit > 0;
  const limit = limited ? SizeFormatter.sizeFormat(server.trafficLimit) : t('unlimited');
  return (
    <>
      <Tag icon={server.linked ? <LinkOutlined /> : undefined}>
        {/* Unlinked servers carry node id 0 too, which would read as the panel. */}
        <span dir="auto">
          {server.linked ? probeHostLabel(server, t) : t('pages.probe.notLinked')}
        </span>
      </Tag>
      {/* The usage is known only while the server is online. Without a limit the row
          stays, so that every card ends alike. */}
      {server.status === 'online' && (
        <ProbeMeter
          label={t('pages.probe.quota')}
          percent={limited ? (server.trafficUsed / server.trafficLimit) * 100 : null}
          detail={`${SizeFormatter.sizeFormat(server.trafficUsed)} / ${limit}`}
        />
      )}
    </>
  );
}

function Summary({ servers, isMobile }: { servers: ProbeServer[]; isMobile: boolean }) {
  const { t } = useTranslation();
  const totals = {
    online: servers.filter((server) => server.status === 'online').length,
    linked: servers.filter((server) => server.linked).length,
    up: servers.reduce((sum, server) => sum + server.netOut, 0),
    down: servers.reduce((sum, server) => sum + server.netIn, 0),
  };
  return (
    <Card size="small" hoverable className="summary-card">
      <Row gutter={[16, isMobile ? 16 : 12]}>
        <Col xs={12} md={6}>
          <Statistic
            title={t('pages.probe.summaryOnline')}
            value={`${totals.online} / ${servers.length}`}
            prefix={<CheckCircleOutlined style={{ color: 'var(--ant-color-success)' }} />}
          />
        </Col>
        <Col xs={12} md={6}>
          <Statistic
            title={t('pages.probe.summaryLinked')}
            value={String(totals.linked)}
            prefix={<LinkOutlined />}
          />
        </Col>
        <Col xs={12} md={6}>
          <Statistic
            title={t('pages.probe.summaryUp')}
            value={SizeFormatter.speedFormat(totals.up)}
            prefix={<ArrowUpOutlined />}
          />
        </Col>
        <Col xs={12} md={6}>
          <Statistic
            title={t('pages.probe.summaryDown')}
            value={SizeFormatter.speedFormat(totals.down)}
            prefix={<ArrowDownOutlined />}
          />
        </Col>
      </Row>
    </Card>
  );
}

export default function ProbePage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const { overview, loading, fetched, fetchError, refetch } = useProbeQuery();
  const [linksOpen, setLinksOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const pageClass = useMemo(() => {
    const classes = ['probe-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const servers = overview?.servers ?? [];
  // Lite failing with nothing kept from before leaves its server list unknown.
  const unreadable = !!overview?.error && servers.length === 0;

  const refreshButton = (
    <Button type="primary" loading={loading} onClick={() => refetch()}>
      {t('refresh')}
    </Button>
  );

  function body() {
    if (!overview) {
      return (
        <Result
          status="error"
          title={t('somethingWentWrong')}
          subTitle={fetchError}
          extra={refreshButton}
        />
      );
    }
    if (!overview.configured) {
      return (
        <Result
          icon={<RadarChartOutlined style={{ color: 'var(--ant-color-primary)' }} />}
          title={t('pages.probe.notConfigured')}
          subTitle={t('pages.probe.notConfiguredHint')}
          extra={
            <Button type="primary" icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)}>
              {t('pages.probe.settings')}
            </Button>
          }
        />
      );
    }
    if (unreadable) {
      return (
        <Result
          status="error"
          title={t('pages.probe.unreadable')}
          subTitle={overview.error}
          extra={refreshButton}
        />
      );
    }
    return (
      <div className="probe-page-body">
        {(overview.stale || fetchError) && (
          <Alert
            type="warning"
            showIcon
            title={t('pages.probe.staleData', {
              time: TimeFormatter.formatClock(overview.fetchedAt / 1000),
            })}
            description={overview.error || fetchError}
          />
        )}
        <Summary servers={servers} isMobile={isMobile} />
        {servers.length === 0 ? (
          <Card>
            <Empty description={t('pages.probe.empty')} />
          </Card>
        ) : (
          <div className="probe-grid">
            {servers.map((server) => (
              <ProbeServerCard
                key={server.id}
                server={server}
                subtitle={systemLine(server, t)}
                footer={<ServerFooter server={server} />}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader
              title={t('menu.probe')}
              description={t('pages.probe.intro')}
              extra={
                <Space wrap>
                  {overview?.publicUrl && (
                    <Button
                      href={overview.publicUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      icon={<ExportOutlined />}
                    >
                      {t('pages.probe.openPublic')}
                    </Button>
                  )}
                  {/* Without a Lite address there is no server to link a host to. */}
                  <Button
                    icon={<LinkOutlined />}
                    disabled={!overview?.configured}
                    onClick={() => setLinksOpen(true)}
                  >
                    {t('pages.probe.linkNodes')}
                  </Button>
                  <Button icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)}>
                    {t('pages.probe.settings')}
                  </Button>
                </Space>
              }
            />
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? <div className="loading-spacer" /> : body()}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
      <ProbeLinksModal
        open={linksOpen}
        servers={unreadable ? null : servers}
        onClose={() => setLinksOpen(false)}
        onSaved={() => {
          setLinksOpen(false);
          messageApi.success(t('pages.probe.linksSaved'));
        }}
      />
      <ProbeSettingsModal
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSaved={() => {
          setSettingsOpen(false);
          messageApi.success(t('pages.probe.settingsSaved'));
        }}
      />
    </ConfigProvider>
  );
}
