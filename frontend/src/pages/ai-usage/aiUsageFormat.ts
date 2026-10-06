import type { AiUsageQuotaTier, AiUsageQuotaView, AiUsageWindowEstimate } from '@/generated/zod';

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
/** Brand names stay as they are in every language. */
export const AI_TOOL_NAME: Record<AiTool, string> = { claude: 'Claude Code', codex: 'Codex' };

/** A window's span in Unix seconds: "10/6 11:50 – 16:50" within a day, both dates when longer. */
export function formatWindowSpan(start: number, end: number, locale?: string): string {
  const day = (unix: number) =>
    new Date(unix * 1000).toLocaleDateString(locale, { month: 'numeric', day: 'numeric' });
  const time = (unix: number) =>
    new Date(unix * 1000).toLocaleTimeString(locale, { hour: 'numeric', minute: '2-digit' });
  return day(start) === day(end - 1)
    ? `${day(start)} ${time(start)} – ${time(end)}`
    : `${day(start)} ${time(start)} – ${day(end)} ${time(end)}`;
}

const TIER_ORDER = [
  'five_hour',
  'seven_day',
  'seven_day_opus',
  'seven_day_sonnet',
  'seven_day_fable',
  '30_day',
];
function tierRank(name: string): number {
  const i = TIER_ORDER.indexOf(name);
  return i < 0 ? TIER_ORDER.length : i;
}

/** A window of one tool: Pigger Switch's estimate when it sent one, and the plan's reading. */
export interface WindowSlot {
  tier: string;
  estimate: AiUsageWindowEstimate | null;
  reading: AiUsageQuotaTier | null;
}

/** A tool's windows, shortest first; an older Pigger Switch sends no estimates, so the bare readings. */
export function windowsForTool(quota: AiUsageQuotaView | undefined): WindowSlot[] {
  if (!quota?.success) return [];
  const readings = new Map(quota.tiers.map((t) => [t.name, t]));
  const estimates = quota.estimates?.windows ?? [];
  const slots: WindowSlot[] =
    estimates.length > 0
      ? estimates.map((e) => ({ tier: e.tier, estimate: e, reading: readings.get(e.tier) ?? null }))
      : quota.tiers.map((t) => ({ tier: t.name, estimate: null, reading: t }));
  return slots.sort((a, b) => tierRank(a.tier) - tierRank(b.tier));
}

/** The local date as YYYY-MM-DD. */
export function localDay(now: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** Days a period covers up to today, for a per-day average; "all" starts at the first data day. */
export function periodDays(
  period: 'today' | 'week' | 'month' | 'all',
  periodStart: string,
  firstDay: string | undefined,
  today: string,
): number {
  const start = period === 'today' ? today : period === 'all' ? firstDay || today : periodStart;
  const ms = Date.parse(`${today}T00:00:00Z`) - Date.parse(`${start}T00:00:00Z`);
  return Number.isFinite(ms) ? Math.max(1, Math.round(ms / 86_400_000) + 1) : 1;
}
