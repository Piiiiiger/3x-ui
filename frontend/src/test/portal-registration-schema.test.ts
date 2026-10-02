import { describe, expect, it } from 'vitest';

import { PortalRegistrationSchema } from '@/schemas/portal';

describe('portal registration validation', () => {
  it('uses UTF-8 bytes for the password limit accepted by the server', () => {
    const password = PortalRegistrationSchema.shape.password;
    expect(password.safeParse('密码').success).toBe(true);
    expect(password.safeParse('密'.repeat(24)).success).toBe(true);
    expect(password.safeParse('密'.repeat(25)).success).toBe(false);
    expect(password.safeParse('12345').success).toBe(false);
  });
  it('counts username characters consistently with the server', () => {
    const username = PortalRegistrationSchema.shape.username;
    expect(username.safeParse('🐷🐷').success).toBe(true);
    expect(username.safeParse('🐷').success).toBe(false);
    expect(username.safeParse('🐷'.repeat(33)).success).toBe(false);
  });
});
