import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Progress, theme } from 'antd';

import type { PortalProbeServer } from '@/generated/zod';
import { USAGE_CRIT_COLOR, USAGE_WARN_COLOR, usageTierColor } from '@/models/status';
import { IntlUtil, SizeFormatter, TimeFormatter } from '@/utils';
import { regionFlag } from './regionFlag';
import './ProbeServerCard.css';

// The portal's server is exactly what the card draws; the admin's carries the same fields.
export type ProbeCardServer = Omit<PortalProbeServer, 'id'>;
type ProbeCardPing = ProbeCardServer['pings'][number];

// Lite's own thresholds, so a route has the same colour here as on Lite's page.
const LATENCY_GOOD_MAX_MS = 80;
const LATENCY_BAD_MIN_MS = 180;
const ANSWERED_GOOD_MIN_PERCENT = 95;
const ANSWERED_WARN_MIN_PERCENT = 80;

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

function latencyTone(latency: number): 'good' | 'warn' | 'bad' {
  if (latency < 0 || latency >= LATENCY_BAD_MIN_MS) return 'bad';
  return latency > LATENCY_GOOD_MAX_MS ? 'warn' : 'good';
}

function answeredColor(percent: number, goodColor: string): string {
  if (percent >= ANSWERED_GOOD_MIN_PERCENT) return goodColor;
  return percent >= ANSWERED_WARN_MIN_PERCENT ? USAGE_WARN_COLOR : USAGE_CRIT_COLOR;
}

interface ProbeMeterProps {
  label: string;
  // null is a share of something without a limit: an empty bar and no figure.
  percent: number | null;
  detail?: string;
}

// Exported for the admin page, which adds a quota bar of the same make to the footer.
export function ProbeMeter({ label, percent, detail }: ProbeMeterProps) {
  const { token } = theme.useToken();
  const value = percent === null ? undefined : `${percent.toFixed(1)} %`;
  const share = percent ?? 0;
  return (
    <div className="probe-card-meter">
      <div className="probe-card-meter-head">
        <span className="probe-card-meter-name">
          <span className="probe-card-meter-label">{label}</span>
          {detail && <bdi className="probe-card-meter-detail">{detail}</bdi>}
        </span>
        {value && <bdi className="probe-card-meter-value">{value}</bdi>}
      </div>
      <Progress
        aria-label={[label, value ?? detail].filter(Boolean).join(' ')}
        percent={share}
        showInfo={false}
        size="small"
        strokeColor={usageTierColor(share, token.colorPrimary)}
      />
    </div>
  );
}

function PulseIcon() {
  return (
    <svg
      width={18}
      height={10}
      viewBox="0 0 18 10"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M1 5.5h3.5l2-4 3 7.5 2.5-5 1.5 1.5H17" />
    </svg>
  );
}

function PingRoute({ ping }: { ping: ProbeCardPing }) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const answered = 100 - ping.loss;
  const loss = t('pages.probe.packetLoss', { loss: formatLoss(ping.loss) });
  return (
    <div className="probe-card-route" title={loss}>
      <div className="probe-card-route-head">
        <span className="probe-card-route-name" dir="auto">
          {ping.name}
        </span>
        <span className={`probe-card-route-latency is-${latencyTone(ping.latency)}`}>
          <bdi>{ping.latency < 0 ? t('pages.probe.pingTimeout') : `${ping.latency} ms`}</bdi>
        </span>
      </div>
      {/* The loss has no figure of its own: the bar is the share of pings answered. */}
      <div className="probe-card-route-bar" role="img" aria-label={`${ping.name} ${loss}`}>
        <span
          style={{ width: `${answered}%`, background: answeredColor(answered, token.colorSuccess) }}
        />
      </div>
    </div>
  );
}

function NetworkQuality({ pings }: { pings: ProbeCardPing[] }) {
  const { t } = useTranslation();
  return (
    <div className="probe-card-network">
      <div className="probe-card-network-head">
        <span className="probe-card-network-title">
          <PulseIcon />
          {t('pages.probe.networkQuality')}
        </span>
        <span className="probe-card-network-window">{t('pages.probe.lastHour')}</span>
      </div>
      <div className="probe-card-routes">
        {pings.map((ping) => (
          <PingRoute key={ping.id} ping={ping} />
        ))}
      </div>
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
      {server.pings.length > 0 && <NetworkQuality pings={server.pings} />}
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
      <div className="probe-card-main">
        <div className="probe-card-head">
          {server.region && <span className="probe-card-flag">{regionFlag(server.region)}</span>}
          {/* One line each, so that cards line up: the tooltips carry what is cut off. */}
          <span className="probe-card-name" dir="auto" title={server.name}>
            {server.name}
          </span>
          <span className={`probe-card-status is-${server.status}`}>
            <span className="probe-card-dot" aria-hidden="true" />
            {t(STATUS_LABEL_KEYS[server.status])}
          </span>
        </div>
        {subtitle && (
          <div
            className="probe-card-subtitle"
            title={typeof subtitle === 'string' ? subtitle : undefined}
          >
            {subtitle}
          </div>
        )}
        {server.status === 'online' && <OnlineFigures server={server} />}
        {server.status === 'offline' && server.updatedAt > 0 && (
          <p className="probe-card-note">
            {t('pages.probe.lastSeen', {
              time: IntlUtil.formatDate(server.updatedAt, 'gregorian', i18n.language),
            })}
          </p>
        )}
      </div>
      {footer && <div className="probe-card-footer">{footer}</div>}
    </Card>
  );
}
