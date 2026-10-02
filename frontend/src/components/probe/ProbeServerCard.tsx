import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Progress, Tag, theme } from 'antd';

import type { PortalProbeServer } from '@/generated/zod';
import { usageTierColor } from '@/models/status';
import { IntlUtil, SizeFormatter, TimeFormatter } from '@/utils';
import { regionFlag } from './regionFlag';
import './ProbeServerCard.css';

// The portal's server is exactly what the card draws; the admin's carries the same fields.
export type ProbeCardServer = Omit<PortalProbeServer, 'id'>;

const STATUS_LABEL_KEYS: Record<ProbeCardServer['status'], string> = {
  online: 'online',
  offline: 'offline',
  unknown: 'pages.probe.statusUnknown',
  unmonitored: 'pages.probe.statusUnmonitored',
};

function usedPercent(used: number, total: number): number {
  return total > 0 ? (used / total) * 100 : 0;
}

function formatLoss(loss: number): string {
  return `${Number(loss.toFixed(1))} %`;
}

interface ProbeMeterProps {
  label: string;
  percent: number;
  detail?: string;
}

function ProbeMeter({ label, percent, detail }: ProbeMeterProps) {
  const { token } = theme.useToken();
  const value = `${percent.toFixed(1)} %`;
  return (
    <div className="probe-card-meter">
      <div className="probe-card-meter-head">
        <span className="probe-card-meter-name">
          <span className="probe-card-meter-label">{label}</span>
          {detail && <bdi className="probe-card-meter-detail">{detail}</bdi>}
        </span>
        <bdi className="probe-card-meter-value">{value}</bdi>
      </div>
      <Progress
        aria-label={`${label} ${value}`}
        percent={percent}
        showInfo={false}
        size="small"
        strokeColor={usageTierColor(percent, token.colorPrimary)}
      />
    </div>
  );
}

function OnlineFigures({ server }: { server: ProbeCardServer }) {
  const { t } = useTranslation();
  const size = SizeFormatter.sizeFormat;
  return (
    <>
      <div className="probe-card-meters">
        <ProbeMeter label={t('pages.index.cpu')} percent={server.cpu} />
        <ProbeMeter
          label={t('pages.index.memory')}
          percent={usedPercent(server.memUsed, server.memTotal)}
          detail={`${size(server.memUsed)} / ${size(server.memTotal)}`}
        />
        <ProbeMeter
          label={t('pages.index.storage')}
          percent={usedPercent(server.diskUsed, server.diskTotal)}
          detail={`${size(server.diskUsed)} / ${size(server.diskTotal)}`}
        />
      </div>
      <dl className="probe-card-facts">
        <div>
          <dt>{t('pages.index.historyTabLoad')}</dt>
          <dd>
            <bdi>
              {[server.load1, server.load5, server.load15]
                .map((load) => load.toFixed(2))
                .join(' / ')}
            </bdi>
          </dd>
        </div>
        <div>
          <dt>{t('pages.nodes.uptime')}</dt>
          <dd>
            <bdi>{TimeFormatter.formatSecond(server.uptime)}</bdi>
          </dd>
        </div>
        <div>
          <dt>{t('pages.clients.speed')}</dt>
          <dd>
            <bdi>↑ {SizeFormatter.speedFormat(server.netOut)}</bdi>
            <bdi>↓ {SizeFormatter.speedFormat(server.netIn)}</bdi>
          </dd>
        </div>
        <div>
          <dt>{t('pages.probe.totalTraffic')}</dt>
          <dd>
            <bdi>↑ {size(server.netTotalUp)}</bdi>
            <bdi>↓ {size(server.netTotalDown)}</bdi>
          </dd>
        </div>
      </dl>
      {server.pings.length > 0 && (
        <div className="probe-card-pings">
          {server.pings.map((ping) => (
            <Tag key={ping.id}>
              <span dir="auto">{ping.name}</span>{' '}
              <bdi>
                {ping.latency < 0 ? t('pages.probe.pingTimeout') : `${ping.latency} ms`} ·{' '}
                {formatLoss(ping.loss)}
              </bdi>
            </Tag>
          ))}
        </div>
      )}
    </>
  );
}

interface ProbeServerCardProps {
  server: ProbeCardServer;
  subtitle?: ReactNode;
  footer?: ReactNode;
}

export default function ProbeServerCard({ server, subtitle, footer }: ProbeServerCardProps) {
  const { t, i18n } = useTranslation();
  return (
    <Card size="small" className="probe-card">
      <div className="probe-card-head">
        {server.region && <span className="probe-card-flag">{regionFlag(server.region)}</span>}
        <span className="probe-card-name" dir="auto">
          {server.name}
        </span>
        <span className={`probe-card-status is-${server.status}`}>
          <span className="probe-card-dot" aria-hidden="true" />
          {t(STATUS_LABEL_KEYS[server.status])}
        </span>
      </div>
      {subtitle && <div className="probe-card-subtitle">{subtitle}</div>}
      {server.status === 'online' && <OnlineFigures server={server} />}
      {server.status === 'offline' && server.updatedAt > 0 && (
        <p className="probe-card-note">
          {t('pages.probe.lastSeen', {
            time: IntlUtil.formatDate(server.updatedAt, 'gregorian', i18n.language),
          })}
        </p>
      )}
      {footer && <div className="probe-card-footer">{footer}</div>}
    </Card>
  );
}
