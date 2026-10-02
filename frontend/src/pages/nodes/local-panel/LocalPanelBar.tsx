import { lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Modal, Spin, message } from 'antd';
import { CopyOutlined, CloudDownloadOutlined } from '@ant-design/icons';

import { HttpUtil, ClipboardManager, FileManager } from '@/utils';
import {
  USAGE_CRIT_COLOR,
  USAGE_CRIT_PERCENT,
  USAGE_WARN_COLOR,
  USAGE_WARN_PERCENT,
} from '@/models/status';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { LazyMount } from '@/components/utility';
import XrayActionBar from './XrayActionBar';
import type { PanelUpdateInfo } from './PanelUpdateModal';
import './LocalPanelBar.css';

const JsonEditor = lazy(() => import('@/components/form/JsonEditor'));
const PanelUpdateModal = lazy(() => import('./PanelUpdateModal'));
const LogModal = lazy(() => import('./LogModal'));
const SystemHistoryModal = lazy(() => import('./SystemHistoryModal'));
const XrayMetricsModal = lazy(() => import('./XrayMetricsModal'));
const XrayLogModal = lazy(() => import('./XrayLogModal'));
const AmneziaWGLogModal = lazy(() => import('./AmneziaWGLogModal'));
const VersionModal = lazy(() => import('./VersionModal'));

/** This panel's own Xray: its state, controls, logs and config, on the local host's page. */
export default function LocalPanelBar() {
  const { t } = useTranslation();
  const { status, fetched, fetchError, refresh } = useStatusQuery();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();

  const [accessLogEnable, setAccessLogEnable] = useState(false);
  const [devChannelEnable, setDevChannelEnable] = useState(false);
  const [panelUpdateInfo, setPanelUpdateInfo] = useState<PanelUpdateInfo>({
    currentVersion: '',
    latestVersion: '',
    updateAvailable: false,
  });

  const [logsOpen, setLogsOpen] = useState(false);
  const [panelUpdateOpen, setPanelUpdateOpen] = useState(false);
  const [sysHistoryOpen, setSysHistoryOpen] = useState(false);
  const [xrayMetricsOpen, setXrayMetricsOpen] = useState(false);
  const [xrayLogsOpen, setXrayLogsOpen] = useState(false);
  const [amneziawgLogsOpen, setAmneziawgLogsOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [configTextOpen, setConfigTextOpen] = useState(false);
  const [configText, setConfigText] = useState('');
  const [busyTip, setBusyTip] = useState<string | null>(null);

  useEffect(() => {
    HttpUtil.post<{ accessLogEnable?: boolean; devChannelEnable?: boolean }>(
      '/panel/api/setting/defaultSettings',
    ).then((msg) => {
      if (msg?.success && msg.obj) {
        setAccessLogEnable(!!msg.obj.accessLogEnable);
        setDevChannelEnable(!!msg.obj.devChannelEnable);
      }
    });
    HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo').then((msg) => {
      if (msg?.success && msg.obj) setPanelUpdateInfo(msg.obj);
    });
  }, []);

  const displayVersion = useMemo(
    () => window.X_UI_CUR_VER || panelUpdateInfo.currentVersion || '?',
    [panelUpdateInfo.currentVersion],
  );

  const setBusy = useCallback(
    ({ busy, tip }: { busy: boolean; tip?: string }) => {
      setBusyTip(busy ? (tip ?? t('loading')) : null);
    },
    [t],
  );

  const stopXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopXrayService');
    await refresh();
  }, [refresh]);

  const restartXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartXrayService');
    await refresh();
  }, [refresh]);

  async function handleChannelChange(dev: boolean) {
    const res = await HttpUtil.post('/panel/api/server/setUpdateChannel', { dev });
    if (!res?.success) return;
    setDevChannelEnable(dev);
    const msg = await HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo');
    if (msg?.success && msg.obj) setPanelUpdateInfo(msg.obj);
  }

  async function openConfig() {
    setBusy({ busy: true });
    try {
      const msg = await HttpUtil.get('/panel/api/server/getConfigJson');
      if (!msg?.success) return;
      setConfigText(JSON.stringify(msg.obj, null, 2));
      setConfigTextOpen(true);
    } finally {
      setBusy({ busy: false });
    }
  }

  async function copyConfig() {
    const ok = await ClipboardManager.copyText(configText || '');
    if (ok) messageApi.success(t('copied'));
  }

  const health = useMemo(() => {
    const items = [
      { name: t('pages.index.cpu'), value: status.cpu.percent },
      { name: t('pages.index.memory'), value: status.mem.percent },
      { name: t('pages.index.swap'), value: status.swap.percent },
      { name: t('pages.index.storage'), value: status.disk.percent },
    ];
    const list = (xs: typeof items) => xs.map((i) => `${i.name} ${i.value.toFixed(0)}%`).join(', ');
    const crit = items.filter((i) => i.value >= USAGE_CRIT_PERCENT);
    if (crit.length)
      return {
        text: t('pages.index.healthCritical', { list: list(crit) }),
        color: USAGE_CRIT_COLOR,
      };
    const warm = items.filter((i) => i.value >= USAGE_WARN_PERCENT);
    if (warm.length)
      return { text: t('pages.index.healthWarm', { list: list(warm) }), color: USAGE_WARN_COLOR };
    return null;
  }, [status, t]);

  let bar;
  if (!fetched) {
    bar = <Card loading size="small" />;
  } else if (fetchError) {
    bar = (
      <Alert
        type="error"
        showIcon
        title={t('somethingWentWrong')}
        description={fetchError}
        action={
          <Button size="small" onClick={refresh}>
            {t('refresh')}
          </Button>
        }
      />
    );
  } else {
    bar = (
      <XrayActionBar
        status={status}
        isMobile={isMobile}
        accessLogEnable={accessLogEnable}
        panelVersion={displayVersion}
        latestVersion={panelUpdateInfo.latestVersion}
        updateAvailable={panelUpdateInfo.updateAvailable}
        onStopXray={stopXray}
        onRestartXray={restartXray}
        onOpenLogs={() => setLogsOpen(true)}
        onOpenXrayLogs={() => setXrayLogsOpen(true)}
        onOpenAmneziaWGLogs={() => setAmneziawgLogsOpen(true)}
        onOpenConfig={openConfig}
        onOpenSystemHistory={() => setSysHistoryOpen(true)}
        onOpenXrayMetrics={() => setXrayMetricsOpen(true)}
        onOpenPanelUpdate={() => setPanelUpdateOpen(true)}
        onOpenVersionSwitch={() => setVersionOpen(true)}
      />
    );
  }

  return (
    <div className="local-panel">
      {messageContextHolder}
      <Spin fullscreen spinning={busyTip !== null} description={busyTip ?? undefined} />
      {bar}
      {health && (
        <div className="ov-health" style={{ color: health.color }}>
          <span className="ov-health-mark" />
          {health.text}
        </div>
      )}

      <LazyMount when={panelUpdateOpen}>
        <PanelUpdateModal
          open={panelUpdateOpen}
          info={panelUpdateInfo}
          devChannelEnable={devChannelEnable}
          onChannelChange={handleChannelChange}
          onClose={() => setPanelUpdateOpen(false)}
          onBusy={setBusy}
        />
      </LazyMount>
      <LazyMount when={logsOpen}>
        <LogModal open={logsOpen} onClose={() => setLogsOpen(false)} />
      </LazyMount>
      <LazyMount when={sysHistoryOpen}>
        <SystemHistoryModal
          open={sysHistoryOpen}
          status={status}
          onClose={() => setSysHistoryOpen(false)}
        />
      </LazyMount>
      <LazyMount when={xrayMetricsOpen}>
        <XrayMetricsModal open={xrayMetricsOpen} onClose={() => setXrayMetricsOpen(false)} />
      </LazyMount>
      <LazyMount when={xrayLogsOpen}>
        <XrayLogModal open={xrayLogsOpen} onClose={() => setXrayLogsOpen(false)} />
      </LazyMount>
      <LazyMount when={amneziawgLogsOpen}>
        <AmneziaWGLogModal open={amneziawgLogsOpen} onClose={() => setAmneziawgLogsOpen(false)} />
      </LazyMount>
      <LazyMount when={versionOpen}>
        <VersionModal
          open={versionOpen}
          status={status}
          onClose={() => setVersionOpen(false)}
          onBusy={setBusy}
        />
      </LazyMount>

      <LazyMount when={configTextOpen}>
        <Modal
          open={configTextOpen}
          title={t('pages.index.config')}
          width={isMobile ? '100%' : 900}
          style={isMobile ? { top: 20, maxWidth: 'calc(100vw - 16px)' } : { top: 20 }}
          onCancel={() => setConfigTextOpen(false)}
          footer={[
            <Button
              key="download"
              onClick={() => FileManager.downloadTextFile(configText, 'config.json')}
              size={isMobile ? 'small' : 'middle'}
              icon={<CloudDownloadOutlined />}
            >
              {isMobile ? 'Download' : 'config.json'}
            </Button>,
            <Button
              key="copy"
              type="primary"
              onClick={copyConfig}
              size={isMobile ? 'small' : 'middle'}
              icon={<CopyOutlined />}
            >
              Copy
            </Button>,
          ]}
        >
          <JsonEditor
            value={configText}
            onChange={setConfigText}
            minHeight={isMobile ? '300px' : 'calc(100vh - 220px)'}
            maxHeight={isMobile ? '70vh' : 'calc(100vh - 220px)'}
            readOnly
          />
        </Modal>
      </LazyMount>
    </div>
  );
}
