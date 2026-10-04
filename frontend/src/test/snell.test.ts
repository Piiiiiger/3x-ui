import { describe, expect, it } from 'vitest';
import { generateSnellPsk, SnellInboundSettingsSchema } from '@/schemas/protocols/inbound/snell';
import { createDefaultInboundSettings } from '@/lib/xray/inbound-defaults';
import { canEnableStream, canEnableSniffing } from '@/lib/xray/protocol-capabilities';
import { composeInboundTag } from '@/lib/xray/inbound-tag';
import { rawInboundToFormValues, formValuesToWirePayload } from '@/lib/xray/inbound-form-adapter';
import { InboundFormSchema } from '@/schemas/forms/inbound-form';
describe('managed Snell', () => {
  it('generates distinct 256-bit credentials without silently assigning clients', () => {
    const one = generateSnellPsk();
    expect(one).toMatch(/^[0-9a-f]{64}$/);
    expect(generateSnellPsk()).not.toBe(one);
    expect(SnellInboundSettingsSchema.parse(createDefaultInboundSettings('snell')).clients).toEqual(
      [],
    );
  });
  it('rejects INI injection and unsupported versions', () => {
    expect(
      SnellInboundSettingsSchema.safeParse({ psk: '0123456789abcdef\nlisten=anything' }).success,
    ).toBe(false);
    expect(
      SnellInboundSettingsSchema.safeParse({ psk: generateSnellPsk(), version: 4 }).success,
    ).toBe(false);
  });
  it('roundtrips settings through the editor without Xray transport or sniffing', () => {
    const settings = createDefaultInboundSettings('snell');
    const values = rawInboundToFormValues({
      protocol: 'snell',
      settings,
      nodeId: 3,
      port: 26163,
      listen: '0.0.0.0',
    });
    expect(InboundFormSchema.safeParse(values).success).toBe(true);
    const wire = formValuesToWirePayload(values);
    expect(JSON.parse(wire.settings).psk).toBe((settings as { psk: string }).psk);
    expect(canEnableStream(values)).toBe(false);
    expect(canEnableSniffing(values)).toBe(false);
    expect(composeInboundTag({ ...values, nodeId: values.nodeId })).toBe('n3-in-26163-tcpudp');
  });
});
