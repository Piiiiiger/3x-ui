import { z } from 'zod';

export const SnellInboundSettingsSchema = z.object({
  psk: z.string().regex(/^[A-Za-z0-9_+/=-]{16,256}$/, 'pages.inbounds.snell.pskHelp'),
  version: z.literal(5).default(5),
  ipv6: z.boolean().default(false),
  reuse: z.boolean().default(true),
  clients: z.array(z.object({ email: z.string() }).loose()).default([]),
});
export type SnellInboundSettings = z.infer<typeof SnellInboundSettingsSchema>;

export function generateSnellPsk(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(32)), (v) =>
    v.toString(16).padStart(2, '0'),
  ).join('');
}
