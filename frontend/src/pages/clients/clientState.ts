import type { ClientRecord } from '@/hooks/useClients';

export type ClientState = 'enabled' | 'disabled' | 'exhausted' | 'expired';

const DAY_MS = 86_400_000;

export function usedBytes(row: ClientRecord): number {
  return (row.traffic?.up ?? 0) + (row.traffic?.down ?? 0);
}

/** The 状态 column: a client the panel switched off says why, before plain on/off. */
export function clientState(row: ClientRecord, now: number): ClientState {
  const expiry = row.expiryTime ?? 0;
  if (expiry > 0 && expiry <= now) return 'expired';
  const total = row.totalGB ?? 0;
  if (total > 0 && usedBytes(row) >= total) return 'exhausted';
  return row.enable ? 'enabled' : 'disabled';
}

/** Days left, rounded up; once past, minus the whole days since (at least 1); null for none. */
export function daysToExpiry(row: ClientRecord, now: number): number | null {
  const expiry = row.expiryTime ?? 0;
  if (expiry <= 0) return null;
  const diff = expiry - now;
  return diff > 0 ? Math.ceil(diff / DAY_MS) : -Math.max(1, Math.floor(-diff / DAY_MS));
}
