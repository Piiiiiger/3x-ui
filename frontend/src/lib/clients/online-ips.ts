// Slots in use against the IP limit, e.g. "2/3"; a limit of 0 means none.
export function formatIpSlots(count: number, limit: number): string {
  return `${count}/${limit > 0 ? limit : '∞'}`;
}

// Minutes left on a ban, rounded up so a running ban never reads as 0.
export function banMinutesLeft(expiresAt: number, nowMs: number): number {
  return Math.max(1, Math.ceil((expiresAt * 1000 - nowMs) / 60_000));
}
