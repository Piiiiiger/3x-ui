import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Button, Card, Popconfirm, Tag, Tooltip } from 'antd';
import { EditOutlined, ExportOutlined, PoweroffOutlined, ReloadOutlined } from '@ant-design/icons';

import { NetworkQuality, ProbeMeter } from '@/components/probe/ProbeServerCard';
import { SizeFormatter, TimeFormatter } from '@/utils';
import type { HostMeter, HostView } from './hostView';
import { useRelativeTime } from './relativeTime';
import './HostCard.css';

export interface HostCardHandlers {
  onRestartXray: (host: HostView) => void;
  onEdit: (host: HostView) => void;
  onSetEnable: (host: HostView, enable: boolean) => void;
}

interface HostCardProps extends HostCardHandlers {
  host: HostView;
  showAddress: boolean;
}

function meterDetail(meter: HostMeter | null): string | undefined {
  if (!meter || meter.used === undefined || meter.total === undefined) return undefined;
  return `${SizeFormatter.sizeFormat(meter.used)} / ${SizeFormatter.sizeFormat(meter.total)}`;
}

function statusOf(host: HostView): { tone: string; color?: string; key: string } {
  if (!host.enabled) return { tone: 'off', key: 'pages.clients.state.disabled' };
  if (!host.online)
    return { tone: 'offline', color: 'red', key: 'pages.nodes.statusValues.offline' };
  if (host.xrayIssue === 'error') {
    return { tone: 'issue', color: 'purple', key: 'pages.nodes.statusValues.xrayError' };
  }
  if (host.xrayIssue === 'stop') {
    return { tone: 'issue', color: 'purple', key: 'pages.nodes.statusValues.xrayStopped' };
  }
  return { tone: 'online', color: 'green', key: 'pages.nodes.statusValues.online' };
}

const KIND_KEYS: Record<HostView['kind'], string> = {
  local: 'pages.inbounds.localPanel',
  agent: 'pages.nodes.kindAgent',
  panel: 'pages.nodes.kindPanel',
};

/** 服务管理's server card: every card has the same sections, filled or not. */
const HostCard = memo(function HostCard({
  host,
  showAddress,
  onRestartXray,
  onEdit,
  onSetEnable,
}: HostCardProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const relativeTime = useRelativeTime();
  const status = statusOf(host);
  const remote = host.kind !== 'local' && !host.transitive;
  const page = host.transitive ? '' : `/nodes/${host.nodeId ?? 'local'}`;
  const traffic = host.traffic;
  const size = SizeFormatter.sizeFormat;

  return (
    <Card size="small" className={`host-card${host.enabled ? '' : ' is-disabled'}`}>
      <div className="host-card-head">
        <span className={`host-card-dot is-${status.tone}`} aria-hidden="true" />
        <span className="host-card-name" dir="auto" title={host.name}>
          {host.name}
        </span>
        <div className="host-card-actions">
          <Tooltip title={t('pages.nodes.open')}>
            <Button
              size="small"
              icon={<ExportOutlined />}
              aria-label={t('pages.nodes.open')}
              disabled={!page}
              onClick={() => navigate(page)}
            />
          </Tooltip>
          <Tooltip title={t('pages.nodes.restartXray')}>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              aria-label={t('pages.nodes.restartXray')}
              disabled={!host.enabled || !host.online || host.transitive}
              onClick={() => onRestartXray(host)}
            />
          </Tooltip>
          {remote && (
            <Tooltip title={t('edit')}>
              <Button
                size="small"
                icon={<EditOutlined />}
                aria-label={t('edit')}
                onClick={() => onEdit(host)}
              />
            </Tooltip>
          )}
          {remote &&
            (host.enabled ? (
              <Popconfirm
                title={t('pages.nodes.disableConfirm', { name: host.name })}
                okText={t('pages.clients.disable')}
                okType="danger"
                cancelText={t('cancel')}
                onConfirm={() => onSetEnable(host, false)}
              >
                <Tooltip title={t('pages.clients.disable')}>
                  <Button
                    size="small"
                    danger
                    icon={<PoweroffOutlined />}
                    aria-label={t('pages.clients.disable')}
                  />
                </Tooltip>
              </Popconfirm>
            ) : (
              <Tooltip title={t('pages.clients.enable')}>
                <Button
                  size="small"
                  icon={<PoweroffOutlined />}
                  aria-label={t('pages.clients.enable')}
                  onClick={() => onSetEnable(host, true)}
                />
              </Tooltip>
            ))}
        </div>
      </div>

      <div className="host-card-tags">
        <Tag>{t(KIND_KEYS[host.kind])}</Tag>
        <Tag color={status.color}>{t(status.key)}</Tag>
        <span className="host-card-fact">
          {t('pages.nodes.nodesCount', { enabled: host.nodesEnabled, total: host.nodesTotal })}
        </span>
        <span className="host-card-fact">Xray {host.xrayVersion || '—'}</span>
      </div>

      <div className="host-card-address" dir="ltr">
        {host.address ? (
          <span className={showAddress ? 'address-visible' : 'address-hidden'}>{host.address}</span>
        ) : (
          '—'
        )}
      </div>

      <div className="probe-card-meters">
        <ProbeMeter label={t('pages.index.cpu')} percent={host.cpu?.percent ?? null} />
        <ProbeMeter
          label={t('pages.index.memory')}
          percent={host.mem?.percent ?? null}
          detail={meterDetail(host.mem)}
        />
        <ProbeMeter
          label={t('pages.index.storage')}
          percent={host.disk?.percent ?? null}
          detail={meterDetail(host.disk)}
        />
        <ProbeMeter
          label={t('pages.probe.quota')}
          percent={traffic && traffic.limit > 0 ? (traffic.used / traffic.limit) * 100 : null}
          detail={
            traffic
              ? `${size(traffic.used)} / ${traffic.limit > 0 ? size(traffic.limit) : t('unlimited')}`
              : '—'
          }
        />
      </div>

      <dl className="probe-card-facts">
        <div>
          <dt>{t('pages.nodes.liveSpeed')}</dt>
          <dd>
            {host.speed ? (
              <>
                <bdi>↑ {SizeFormatter.speedFormat(host.speed.up)}</bdi>
                <bdi>↓ {SizeFormatter.speedFormat(host.speed.down)}</bdi>
              </>
            ) : (
              <bdi>—</bdi>
            )}
          </dd>
        </div>
        <div>
          <dt>{t('pages.nodes.uptime')}</dt>
          <dd>
            <bdi>{host.uptimeSecs > 0 ? TimeFormatter.formatSecond(host.uptimeSecs) : '—'}</bdi>
          </dd>
        </div>
      </dl>

      <div className="host-card-network">
        {host.pings && host.pings.length > 0 ? (
          <NetworkQuality pings={host.pings} />
        ) : (
          <div className="host-card-placeholder">
            {host.pings ? t('pages.probe.statusUnknown') : t('pages.nodes.probeNotLinked')}
          </div>
        )}
      </div>

      <div className="host-card-foot">
        <span>{t('pages.nodes.lastHeartbeat')}</span>
        <span>{host.kind === 'local' ? '—' : relativeTime(host.lastHeartbeat)}</span>
      </div>
    </Card>
  );
});

export default HostCard;
