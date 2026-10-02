import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Checkbox,
  ConfigProvider,
  Input,
  Layout,
  Modal,
  Result,
  Segmented,
  Spin,
  Typography,
  message,
} from 'antd';
import {
  AppstoreOutlined,
  CloudDownloadOutlined,
  ExportOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  LinkOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { useNodeMutations } from '@/api/queries/useNodeMutations';
import { useProbeQuery } from '@/api/queries/useProbeQuery';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import type { ProbeServer } from '@/generated/zod';
import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import ProbeLinksModal from '@/pages/probe/ProbeLinksModal';
import ProbeSettingsModal from '@/pages/probe/ProbeSettingsModal';
import NodeList from './NodeList';
import HostCard from './HostCard';
import { LocalPanelCard, nodesByHostOf } from './HostNodeChips';
import { localHostView, remoteHostView, type HostView } from './hostView';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import NodeFormModal from './NodeFormModal';
import { setMessageInstance } from '@/utils/messageBus';
import { HttpUtil, TimeFormatter } from '@/utils';
import { formatPanelVersion } from '@/lib/panel-version';
import type { PanelUpdateInfo } from './local-panel/PanelUpdateModal';
import './HostCard.css';
import './NodesPage.css';

type HostsView = 'grid' | 'list';
const VIEW_KEY = 'hosts-view';
const ADDRESS_KEY = 'hosts-show-address';

function readStored(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function store(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // A remembered choice is a convenience; private windows may refuse storage.
  }
}

// Confirm-dialog body that lets the operator pick the stable or dev channel for
// a node panel update. Reports changes via onChange so the imperative
// modal.confirm onOk can read the latest choice through a ref.
function UpdateChannelChoice({ onChange }: { onChange: (dev: boolean) => void }) {
  const { t } = useTranslation();
  const [dev, setDev] = useState(false);
  return (
    <div>
      <p>{t('pages.nodes.updateConfirmContent')}</p>
      <Checkbox
        checked={dev}
        onChange={(e) => {
          setDev(e.target.checked);
          onChange(e.target.checked);
        }}
      >
        {t('pages.nodes.updateDevChannel')}
      </Checkbox>
      {dev && (
        <Alert
          type="info"
          showIcon
          style={{ marginTop: 8 }}
          title={t('pages.index.devChannelWarning')}
        />
      )}
    </div>
  );
}

/** 主机: the panel's hosts as 妙妙屋X's 服务管理 cards, with the probe's figures on them. */
export default function NodesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const { nodes, loading, fetched, fetchError, refetch, totals } = useNodesQuery();
  const { data: inboundOptions } = useInboundOptions();
  const nodesByHost = useMemo(() => nodesByHostOf(inboundOptions ?? []), [inboundOptions]);
  const {
    overview,
    loading: probeLoading,
    fetchError: probeError,
    refetch: refetchProbe,
  } = useProbeQuery();
  const { status } = useStatusQuery();
  const {
    create,
    update,
    remove,
    setEnable,
    testConnection,
    fetchFingerprint,
    fetchInbounds,
    probe,
    updatePanels,
    mintAgentSecret,
  } = useNodeMutations();

  const { data: latestVersion = '' } = useQuery({
    queryKey: ['server', 'panelUpdateInfo'],
    queryFn: async () => {
      const msg = await HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo');
      return msg?.obj?.latestVersion || '';
    },
    staleTime: 5 * 60 * 1000,
  });

  const [view, setView] = useState<HostsView>(() =>
    readStored(VIEW_KEY) === 'list' ? 'list' : 'grid',
  );
  const [showAddress, setShowAddress] = useState(() => readStored(ADDRESS_KEY) === 'true');
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [formNode, setFormNode] = useState<NodeRecord | null>(null);
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [linksOpen, setLinksOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [mtlsOpen, setMtlsOpen] = useState(false);
  const [trustCa, setTrustCa] = useState('');
  const [copyingCa, setCopyingCa] = useState(false);
  const [savingTrustCa, setSavingTrustCa] = useState(false);

  const changeView = useCallback((next: HostsView) => {
    setView(next);
    store(VIEW_KEY, next);
  }, []);

  const changeShowAddress = useCallback((next: boolean) => {
    setShowAddress(next);
    store(ADDRESS_KEY, String(next));
  }, []);

  const probeServers = useMemo(() => overview?.servers ?? [], [overview]);
  // Lite failing with nothing kept from before leaves its server list unknown.
  const probeUnreadable = !!overview?.error && probeServers.length === 0;
  const probeByHost = useMemo(() => {
    const byHost = new Map<number, ProbeServer>();
    for (const server of probeServers) if (server.linked) byHost.set(server.nodeId, server);
    return byHost;
  }, [probeServers]);

  const hosts = useMemo<HostView[]>(
    () => [
      localHostView(
        status,
        probeByHost.get(0),
        nodesByHost.get(0) ?? [],
        t('pages.inbounds.localPanel'),
        window.X_UI_CUR_VER ? formatPanelVersion(window.X_UI_CUR_VER) : '',
      ),
      ...nodes.map((node) =>
        remoteHostView(
          node,
          node.transitive ? undefined : probeByHost.get(node.id),
          node.transitive ? [] : (nodesByHost.get(node.id) ?? []),
        ),
      ),
    ],
    [status, probeByHost, nodesByHost, nodes, t],
  );

  const onCopyNodeCa = useCallback(async () => {
    setCopyingCa(true);
    try {
      const msg = await HttpUtil.post<{ caCert: string }>('/panel/api/nodes/mtls/ca');
      const ca = msg?.obj?.caCert;
      if (msg?.success && ca) {
        await navigator.clipboard.writeText(ca);
        messageApi.success(t('pages.nodes.mtls.caCopied'));
      } else {
        messageApi.error(msg?.msg || t('pages.nodes.mtls.caFailed'));
      }
    } catch {
      messageApi.error(t('pages.nodes.mtls.caFailed'));
    } finally {
      setCopyingCa(false);
    }
  }, [messageApi, t]);

  const onSaveTrustCa = useCallback(async () => {
    setSavingTrustCa(true);
    try {
      const msg = await HttpUtil.post('/panel/api/nodes/mtls/trustCA', { caCert: trustCa });
      if (msg?.success) {
        messageApi.success(t('pages.nodes.mtls.saved'));
        setMtlsOpen(false);
      } else {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
      }
    } catch {
      messageApi.error(t('somethingWentWrong'));
    } finally {
      setSavingTrustCa(false);
    }
  }, [trustCa, messageApi, t]);

  const onAdd = useCallback(() => {
    setFormMode('add');
    setFormNode(null);
    setFormOpen(true);
  }, []);

  const onEdit = useCallback((node: NodeRecord) => {
    setFormMode('edit');
    setFormNode({ ...node });
    setFormOpen(true);
  }, []);

  const onSave = useCallback(
    async (payload: Partial<NodeRecord>) => {
      if (formMode === 'edit' && formNode?.id) {
        return update(formNode.id, payload);
      }
      return create(payload);
    },
    [formMode, formNode, update, create],
  );

  const onDelete = useCallback(
    (node: NodeRecord) => {
      modal.confirm({
        title: t('pages.nodes.deleteConfirmTitle', { name: node.name }),
        content: t('pages.nodes.deleteConfirmContent'),
        okText: t('delete'),
        okType: 'danger',
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await remove(node.id);
          if (msg?.success) messageApi.success(t('pages.nodes.toasts.deleted'));
        },
      });
    },
    [modal, t, remove, messageApi],
  );

  const onProbe = useCallback(
    async (node: NodeRecord) => {
      const msg = await probe(node.id);
      if (msg?.success && msg.obj) {
        if (msg.obj.status === 'online') {
          // Even if xray is in error/stop on the node we still reached its panel API.
          messageApi.success(t('pages.nodes.connectionOk', { ms: msg.obj.latencyMs }));
        } else {
          messageApi.error(msg.obj.error || t('pages.nodes.toasts.probeFailed'));
        }
      }
      // Refresh the list so the new xrayState / xrayError (if any) appears immediately in the row.
      refetch();
    },
    [probe, t, messageApi, refetch],
  );

  const onToggleEnable = useCallback(
    async (node: NodeRecord, next: boolean) => {
      await setEnable(node.id, next);
    },
    [setEnable],
  );

  const nodeOf = useCallback(
    (host: HostView) => nodes.find((node) => node.id === host.nodeId && !node.transitive),
    [nodes],
  );

  const onEditHost = useCallback(
    (host: HostView) => {
      const node = nodeOf(host);
      if (node) onEdit(node);
    },
    [nodeOf, onEdit],
  );

  const onSetHostEnable = useCallback(
    (host: HostView, enable: boolean) => {
      const node = nodeOf(host);
      if (node) onToggleEnable(node, enable);
    },
    [nodeOf, onToggleEnable],
  );

  const onRestartXray = useCallback(
    (host: HostView) => {
      modal.confirm({
        title: t('pages.nodes.restartXrayConfirm', { name: host.name }),
        content: t('pages.nodes.restartXrayHint'),
        okText: t('pages.nodes.restartXray'),
        cancelText: t('cancel'),
        onOk: async () => {
          await HttpUtil.post(
            host.nodeId === null
              ? '/panel/api/server/restartXrayService'
              : `/panel/api/nodes/restartXray/${host.nodeId}`,
          );
          refetch();
        },
      });
    },
    [modal, t, refetch],
  );

  const devRef = useRef(false);

  const runUpdate = useCallback(
    async (ids: number[], dev: boolean) => {
      const msg = await updatePanels(ids, dev);
      if (!msg?.success) {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
        return;
      }
      const results = msg.obj ?? [];
      const ok = results.filter((r) => r.ok).length;
      const failed = results.length - ok;
      if (failed === 0) {
        messageApi.success(t('pages.nodes.toasts.updateStarted'));
      } else {
        const firstError = results.find((r) => !r.ok)?.error ?? '';
        const base = t('pages.nodes.toasts.updateResult', { ok, failed });
        messageApi.warning(firstError ? `${base} — ${firstError}` : base);
      }
      setSelectedIds([]);
    },
    [updatePanels, messageApi, t],
  );

  const onUpdateNode = useCallback(
    (node: NodeRecord) => {
      devRef.current = false;
      modal.confirm({
        title: t('pages.nodes.updateConfirmTitle', { count: 1 }),
        content: (
          <UpdateChannelChoice
            onChange={(v) => {
              devRef.current = v;
            }}
          />
        ),
        okText: t('update'),
        cancelText: t('cancel'),
        onOk: () => runUpdate([node.id], devRef.current),
      });
    },
    [modal, t, runUpdate],
  );

  const onUpdateSelected = useCallback(() => {
    const eligible = nodes
      .filter((n) => selectedIds.includes(n.id) && n.enable && n.status === 'online')
      .map((n) => n.id);
    if (eligible.length === 0) {
      messageApi.warning(t('pages.nodes.toasts.updateNoneEligible'));
      return;
    }
    devRef.current = false;
    modal.confirm({
      title: t('pages.nodes.updateConfirmTitle', { count: eligible.length }),
      content: (
        <UpdateChannelChoice
          onChange={(v) => {
            devRef.current = v;
          }}
        />
      ),
      okText: t('update'),
      cancelText: t('cancel'),
      onOk: () => runUpdate(eligible, devRef.current),
    });
  }, [modal, t, nodes, selectedIds, runUpdate, messageApi]);

  const pageClass = useMemo(() => {
    const classes = ['nodes-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const settingsButton = (
    <Button icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)}>
      {t('pages.probe.settingsTitle')}
    </Button>
  );

  function probeNotice() {
    if (overview && !overview.configured) {
      return (
        <Alert
          type="info"
          showIcon
          title={t('pages.probe.notConfigured')}
          description={t('pages.probe.notConfiguredHint')}
          action={settingsButton}
        />
      );
    }
    if (probeUnreadable) {
      return (
        <Alert
          type="error"
          showIcon
          title={t('pages.probe.unreadable')}
          description={overview?.error}
        />
      );
    }
    if (overview && (overview.stale || probeError)) {
      return (
        <Alert
          type="warning"
          showIcon
          title={t('pages.probe.staleData', {
            time: TimeFormatter.formatClock(overview.fetchedAt / 1000),
          })}
          description={overview.error || probeError}
        />
      );
    }
    if (!overview && probeError) {
      return (
        <Alert
          type="error"
          showIcon
          title={probeError}
          action={
            <Button loading={probeLoading} onClick={() => refetchProbe()}>
              {t('refresh')}
            </Button>
          }
        />
      );
    }
    return null;
  }

  const toolbar = (
    <div className="hosts-toolbar">
      <Segmented<HostsView>
        value={view}
        onChange={changeView}
        options={[
          { value: 'grid', icon: <AppstoreOutlined />, title: t('pages.plans.viewGrid') },
          { value: 'list', icon: <UnorderedListOutlined />, title: t('pages.plans.viewList') },
        ]}
      />
      <Button
        icon={showAddress ? <EyeInvisibleOutlined /> : <EyeOutlined />}
        onClick={() => changeShowAddress(!showAddress)}
      >
        {showAddress ? t('pages.nodes.hideIp') : t('pages.nodes.showIp')}
      </Button>
      <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>
        {t('pages.nodes.addNode')}
      </Button>
      {/* Without a Lite address there is no server to link a host to. */}
      <Button
        icon={<LinkOutlined />}
        disabled={!overview?.configured}
        onClick={() => setLinksOpen(true)}
      >
        {t('pages.probe.linkNodes')}
      </Button>
      {settingsButton}
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
      <Button icon={<SafetyCertificateOutlined />} onClick={() => setMtlsOpen(true)}>
        {t('pages.nodes.mtls.title')}
      </Button>
      {view === 'list' && selectedIds.length > 0 && (
        <Button icon={<CloudDownloadOutlined />} onClick={onUpdateSelected}>
          {t('pages.nodes.updateSelected', { count: selectedIds.length })}
        </Button>
      )}
      <span className="hosts-toolbar-counters">
        <span className="hosts-counter">
          <span className="hosts-counter-dot is-online" />
          {t('pages.nodes.onlineNodes')} <strong>{totals.online}</strong>
        </span>
        <span className="hosts-counter">
          <span className="hosts-counter-dot is-offline" />
          {t('pages.nodes.offlineNodes')} <strong>{totals.offline}</strong>
        </span>
      </span>
    </div>
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppNav />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader title={t('menu.nodes')} description={t('pages.nodes.intro')} />
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={loading} onClick={() => refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <div className="hosts-body">
                  {toolbar}
                  {probeNotice()}
                  {view === 'grid' ? (
                    <div className="host-grid">
                      {hosts.map((host) => (
                        <HostCard
                          key={host.key}
                          host={host}
                          showAddress={showAddress}
                          onRestartXray={onRestartXray}
                          onEdit={onEditHost}
                          onSetEnable={onSetHostEnable}
                        />
                      ))}
                    </div>
                  ) : (
                    <>
                      <LocalPanelCard nodes={nodesByHost.get(0) ?? []} />
                      <NodeList
                        nodes={nodes}
                        nodesByHost={nodesByHost}
                        loading={loading}
                        isMobile={isMobile}
                        latestVersion={latestVersion}
                        showAddress={showAddress}
                        onShowAddressChange={changeShowAddress}
                        selectedIds={selectedIds}
                        onSelectionChange={setSelectedIds}
                        onEdit={onEdit}
                        onDelete={onDelete}
                        onProbe={onProbe}
                        onToggleEnable={onToggleEnable}
                        onUpdateNode={onUpdateNode}
                      />
                    </>
                  )}
                </div>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <NodeFormModal
          open={formOpen}
          mode={formMode}
          node={formNode}
          testConnection={testConnection}
          fetchFingerprint={fetchFingerprint}
          fetchInbounds={fetchInbounds}
          save={onSave}
          mintAgentSecret={mintAgentSecret}
          onOpenChange={setFormOpen}
        />

        <ProbeLinksModal
          open={linksOpen}
          servers={probeUnreadable ? null : probeServers}
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

        <Modal
          open={mtlsOpen}
          title={t('pages.nodes.mtls.title')}
          footer={null}
          onCancel={() => setMtlsOpen(false)}
          destroyOnHidden
        >
          <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
            {t('pages.nodes.mtls.intro')}
          </Typography.Paragraph>
          <Button onClick={onCopyNodeCa} loading={copyingCa} style={{ marginBottom: 4 }}>
            {t('pages.nodes.mtls.copyCa')}
          </Button>
          <Typography.Paragraph type="secondary">
            {t('pages.nodes.mtls.copyCaHint')}
          </Typography.Paragraph>
          <Typography.Text strong>{t('pages.nodes.mtls.trustLabel')}</Typography.Text>
          <Input.TextArea
            rows={5}
            value={trustCa}
            onChange={(e) => setTrustCa(e.target.value)}
            placeholder={t('pages.nodes.mtls.trustPlaceholder')}
            style={{ marginTop: 4, fontFamily: 'monospace' }}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
            {t('pages.nodes.mtls.trustHint')}
          </Typography.Paragraph>
          <Button type="primary" onClick={onSaveTrustCa} loading={savingTrustCa} block>
            {t('pages.nodes.mtls.save')}
          </Button>
        </Modal>
      </Layout>
    </ConfigProvider>
  );
}
