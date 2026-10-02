const REGIONAL_INDICATOR_A = 0x1f1e6;

// Lite sends a flag emoji, except for a region an admin set by hand: that one
// arrives as its two-letter country code.
export function regionFlag(region: string): string {
  const code = region.toUpperCase();
  if (!/^[A-Z]{2}$/.test(code)) return region;
  return String.fromCodePoint(
    ...Array.from(code, (letter) => REGIONAL_INDICATOR_A + letter.charCodeAt(0) - 65),
  );
}
