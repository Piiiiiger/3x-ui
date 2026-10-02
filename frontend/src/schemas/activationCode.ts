import { z } from 'zod';

export const ActivationCodeFormSchema = z.object({
  count: z.number().int().min(1).max(200),
  quotaGB: z
    .number()
    .min(0)
    .max(Number.MAX_SAFE_INTEGER / 1024 ** 3),
  days: z.number().int().min(0).max(36500),
  resetDay: z.number().int().min(0).max(31),
  note: z.string().max(256),
});
export type ActivationCodeFormValues = z.infer<typeof ActivationCodeFormSchema>;
