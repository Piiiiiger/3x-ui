const DAY_MS = 24 * 60 * 60 * 1000;

// A reset day past the month's end falls on the 1st of the next month, as on Lite.
function resetDateIn(year: number, month: number, resetDay: number): Date {
  const lastDay = new Date(year, month + 1, 0).getDate();
  return resetDay <= lastDay ? new Date(year, month, resetDay) : new Date(year, month + 1, 1);
}

function calendarDays(from: Date, to: Date): number {
  const a = Date.UTC(from.getFullYear(), from.getMonth(), from.getDate());
  const b = Date.UTC(to.getFullYear(), to.getMonth(), to.getDate());
  return Math.max(0, Math.round((b - a) / DAY_MS));
}

/** Days until Lite's monthly reset (0 is today), counted the way Lite's own page counts them. */
export function daysUntilReset(resetDay: number, now: Date): number | null {
  if (!Number.isInteger(resetDay) || resetDay < 1 || resetDay > 31) return null;
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const thisMonth = resetDateIn(today.getFullYear(), today.getMonth(), resetDay);
  const lastMonth = resetDateIn(today.getFullYear(), today.getMonth() - 1, resetDay);
  // Last month's date can roll over into this month's 1st, which is a reset today too.
  if (today.getTime() === thisMonth.getTime() || today.getTime() === lastMonth.getTime()) return 0;
  if (today < thisMonth) return calendarDays(today, thisMonth);
  return calendarDays(today, resetDateIn(today.getFullYear(), today.getMonth() + 1, resetDay));
}
