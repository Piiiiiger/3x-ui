import { z } from 'zod';

// A plan is what its users share: servers, a rule template and an IP limit.
export const PlanFormSchema = z.object({
  name: z.string().trim().min(1, 'pages.plans.errNameRequired'),
  limitIp: z.number().int().min(0),
  remark: z.string(),
  // 0 leaves the plan on the default rule template.
  templateId: z.number().int().min(0),
  inboundIds: z.array(z.number().int().positive()),
});
export type PlanFormValues = z.infer<typeof PlanFormSchema>;
