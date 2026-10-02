import { z } from 'zod';
import { DnsObjectSchema } from './dns';
import { BalancerObjectSchema, RuleObjectSchema } from './routing';

export const XraySettingsValueSchema = z
  .object({
    inbounds: z.array(z.unknown()).optional(),
    outbounds: z
      .array(
        z
          .object({
            tag: z.string().optional(),
            protocol: z.string().optional(),
            settings: z.unknown().optional(),
            streamSettings: z.unknown().optional(),
          })
          .loose(),
      )
      .optional(),
    routing: z
      .object({
        rules: z.array(RuleObjectSchema).optional(),
        balancers: z.array(BalancerObjectSchema).optional(),
        domainStrategy: z.string().optional(),
      })
      .loose()
      .optional(),
    dns: DnsObjectSchema.optional(),
    log: z.record(z.string(), z.unknown()).optional(),
    policy: z
      .object({
        system: z.record(z.string(), z.boolean()).optional(),
        levels: z.record(z.string(), z.record(z.string(), z.unknown())).optional(),
      })
      .loose()
      .optional(),
    observatory: z.unknown().optional(),
    burstObservatory: z.unknown().optional(),
    fakedns: z.unknown().optional(),
  })
  .loose();

export const XrayConfigPayloadSchema = z
  .object({
    xraySetting: XraySettingsValueSchema,
    geodataSources: z.array(z.object({ url: z.string(), file: z.string() })).optional(),
  })
  .loose();

export type XraySettingsValue = z.infer<typeof XraySettingsValueSchema>;
export type XrayConfigPayload = z.infer<typeof XrayConfigPayloadSchema>;
