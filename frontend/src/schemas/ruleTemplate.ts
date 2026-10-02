import { z } from 'zod';

export const RuleTemplateFormSchema = z.object({
  name: z.string().trim().min(1, 'pages.rules.errNameRequired'),
  content: z.string().refine((value) => value.trim() !== '', 'pages.rules.errContentRequired'),
});
export type RuleTemplateFormValues = z.infer<typeof RuleTemplateFormSchema>;
