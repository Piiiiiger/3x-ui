import { z } from 'zod';

import { TrafficDaySchema } from '@/generated/zod';

export const PortalLoginSchema = z.object({
  username: z.string().trim().min(1, 'subscription.portal.required'),
  password: z.string().min(1, 'subscription.portal.required'),
});
export type PortalLoginValues = z.infer<typeof PortalLoginSchema>;

export const PortalPlanSchema = z.object({
  name: z.string(),
  totalGB: z.number(),
  durationDays: z.number(),
  trafficReset: z.string(),
  trafficResetDay: z.number(),
  limitIp: z.number(),
});
export type PortalPlan = z.infer<typeof PortalPlanSchema>;

// What {subPath}portal/data returns for the signed-in client; page is the same
// payload the subscription page is rendered from, or null without a subscription.
export const PortalDataSchema = z.object({
  email: z.string(),
  page: z.custom<SubPageData>((value) => typeof value === 'object').nullable(),
  plan: PortalPlanSchema.nullable(),
  daily: z.array(TrafficDaySchema),
  // Whether to offer the probe view. A server older than that view sends no flag.
  probe: z.boolean().default(false),
});
export type PortalData = z.infer<typeof PortalDataSchema>;
