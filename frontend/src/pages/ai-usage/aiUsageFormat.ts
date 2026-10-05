import type { AiUsageQuotaTier, AiUsageQuotaView } from '@/generated/zod';

const usd = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatUsd(value: number): string {
  if (value > 0 && value < 0.005) return '<$0.01';
  return usd.format(value);
}

export function formatTokens(value: number): string {
  if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
  if (value >= 1e6) return `${(value / 1e6).toFixed(1)}M`;
  if (value >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
  return String(Math.round(value));
}

export function projectName(path: string): string {
  const trimmed = path.replace(/[\\/]+$/, '');
  const parts = trimmed.split(/[\\/]/);
  return parts[parts.length - 1] || trimmed;
}

/** Share of the input that came from the cache; null when there was no input at all. */
export function cacheHitRate(t: {
  inputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
}): number | null {
  const input = t.inputTokens + t.cacheReadTokens + t.cacheWriteTokens;
  return input > 0 ? t.cacheReadTokens / input : null;
}

export function resetCountdown(resetsAt: string, nowMs: number): string | null {
  const at = Date.parse(resetsAt);
  if (!resetsAt || Number.isNaN(at) || at <= nowMs) return null;
  const minutes = Math.floor((at - nowMs) / 60_000);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  if (days > 0) return `${days}d ${hours % 24}h`;
  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  return `${Math.max(1, minutes)}m`;
}

/** The plan's last day as a local YYYY-MM-DD, or null when the reading has none. */
export function planEndDate(iso: string): string | null {
  const at = iso ? new Date(iso) : null;
  if (!at || Number.isNaN(at.getTime())) return null;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;
}

export const AI_TOOLS = ['claude', 'codex'] as const;
export type AiTool = (typeof AI_TOOLS)[number];

const TIER_ORDER = [
  'five_hour',
  'seven_day',
  'seven_day_opus',
  'seven_day_sonnet',
  'seven_day_fable',
  '30_day',
];
const MIN_TIER_ROWS = 2;

export interface QuotaSlot {
  tool: AiTool;
  quota: AiUsageQuotaView | null;
  tiers: (AiUsageQuotaTier | null)[];
}

function tierRank(name: string): number {
  const i = TIER_ORDER.indexOf(name);
  return i < 0 ? TIER_ORDER.length : i;
}

/** One card per tool, each padded to the same number of window rows. */
export function quotaSlots(quotas: AiUsageQuotaView[]): QuotaSlot[] {
  const byTool = new Map(quotas.map((q) => [q.tool, q]));
  const sorted = AI_TOOLS.map((tool) => {
    const quota = byTool.get(tool) ?? null;
    const tiers = quota?.success
      ? [...quota.tiers].sort((a, b) => tierRank(a.name) - tierRank(b.name))
      : [];
    return { tool, quota, tiers };
  });
  const rows = Math.max(MIN_TIER_ROWS, ...sorted.map((s) => s.tiers.length));
  return sorted.map((s) => ({
    ...s,
    tiers: [...s.tiers, ...Array<null>(rows - s.tiers.length).fill(null)],
  }));
}
