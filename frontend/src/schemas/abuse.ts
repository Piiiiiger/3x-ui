import { z } from 'zod';

// Counts and minutes of the abuse checks; 0 turns a check off. Windows stay
// within the hour a server remembers, as the server keeps them anyway.
const count = z.number().int().min(0);
const minutes = z.number().int().min(0).max(60);
const action = z.enum(['record', 'warn', 'ban']);

export const AbuseSettingsFormSchema = z.object({
  rules: z.object({
    spamAttempts: count,
    spamWindowMin: minutes,
    btAttempts: count,
    btWindowMin: minutes,
    scanIps: count,
    scanPortsOnIp: count,
    scanSensitiveIps: count,
    scanWindowMin: minutes,
    floodPerDest: count,
    floodTotal: count,
    crawlerConns: count,
    crawlerHosts: count,
    crawlerWindowMin: minutes,
    crawlerWindows: count,
    speedTestsPerHour: count,
    speedTestsPerDay: count,
    speedTestGapMin: minutes,
    fullSpeedMbps: count,
    fullSpeedWarnMin: count,
    fullSpeedStrikeMin: count,
  }),
  actions: z.object({
    spam: action,
    bt: action,
    scan: action,
    flood: action,
    crawler: action,
    speedtest: action,
    fullspeed: action,
  }),
  signup: z.object({ limit: count, action: z.enum(['record', 'ban']) }),
});
export type AbuseSettingsFormValues = z.infer<typeof AbuseSettingsFormSchema>;
