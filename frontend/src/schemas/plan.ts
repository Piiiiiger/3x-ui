import { z } from 'zod';

export const PlanTrafficResetSchema = z.enum(['never', 'daily', 'weekly', 'monthly']);
export type PlanTrafficReset = z.infer<typeof PlanTrafficResetSchema>;

export const PlanStartSchema = z.enum(['now', 'firstUse', 'keep']);
export type PlanStart = z.infer<typeof PlanStartSchema>;

export const PlanClashModeSchema = z.enum(['inherit', 'custom']);
export type PlanClashMode = z.infer<typeof PlanClashModeSchema>;

// The form edits the quota in GiB; the wire carries bytes, like a client's totalGB.
export const PlanFormSchema = z.object({
  name: z.string().trim().min(1, 'pages.plans.errNameRequired'),
  quotaGB: z.number().min(0),
  durationDays: z.number().int().min(0),
  trafficReset: PlanTrafficResetSchema,
  trafficResetDay: z.number().int().min(1).max(31),
  limitIp: z.number().int().min(0),
  remark: z.string(),
  // Empty inherits the global Clash rules.
  clashRules: z.string(),
  inboundIds: z.array(z.number().int().positive()),
});
export type PlanFormValues = z.infer<typeof PlanFormSchema>;
