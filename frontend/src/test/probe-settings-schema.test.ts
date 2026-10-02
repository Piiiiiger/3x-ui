import { describe, expect, it } from 'vitest';

import { ProbeSettingsFormSchema } from '@/schemas/probe';

function issuesOf(values: { url?: string; publicUrl?: string }): string[] {
  const result = ProbeSettingsFormSchema.safeParse({ url: '', publicUrl: '', ...values });
  return result.success ? [] : result.error.issues.map((issue) => issue.message);
}

// The panel fetches this address from inside its own host: the form turns away
// what the server will refuse, each rule with the input that breaks only it.
describe('ProbeSettingsFormSchema', () => {
  it.each([
    ['a hostname', 'http://localhost:27777'],
    ['a private address', 'http://192.168.1.10:27777'],
    ['a mapped loopback', 'http://[::ffff:127.0.0.1]:27777'],
    ['a path', 'http://127.0.0.1:27777/api'],
    ['a query', 'http://127.0.0.1:27777/?next=1'],
    ['a fragment', 'http://127.0.0.1:27777/#top'],
    ['credentials', 'http://admin:secret@127.0.0.1:27777'],
    ['another scheme', 'ftp://127.0.0.1:27777'],
    ['no scheme', '127.0.0.1:27777'],
  ])('refuses a Lite address with %s', (_rule, url) => {
    expect(issuesOf({ url })).toEqual(['pages.probe.errLiteUrl']);
  });

  it.each([
    ['empty, which turns the probe off', ''],
    ['IPv4 loopback with a port', 'http://127.0.0.1:27777'],
    ['the rest of 127/8', 'http://127.8.9.10'],
    ['IPv6 loopback over TLS', 'https://[::1]:8443'],
  ])('accepts a Lite address that is %s', (_rule, url) => {
    expect(issuesOf({ url })).toEqual([]);
  });

  it.each([
    ['a script URL that names a host', 'javascript://probe.example.com/%0Aalert(1)'],
    ['no scheme', 'probe.example.com'],
    ['a user name', 'https://admin@probe.example.com'],
    ['a password', 'https://:secret@probe.example.com'],
  ])('refuses a public page URL with %s', (_rule, publicUrl) => {
    expect(issuesOf({ publicUrl })).toEqual(['pages.probe.errPublicUrl']);
  });

  it('accepts an http or https public page URL, and none at all', () => {
    expect(issuesOf({ publicUrl: 'https://probe.example.com/all' })).toEqual([]);
    expect(issuesOf({ publicUrl: 'http://probe.example.com' })).toEqual([]);
    expect(issuesOf({ publicUrl: '' })).toEqual([]);
  });

  // The server stores what it is sent after trimming; the form must not refuse padded input.
  it('hands both addresses on without the spaces around them', () => {
    expect(
      ProbeSettingsFormSchema.parse({
        url: '  http://127.0.0.1:27777 ',
        publicUrl: ' https://probe.example.com  ',
      }),
    ).toEqual({ url: 'http://127.0.0.1:27777', publicUrl: 'https://probe.example.com' });
  });
});
