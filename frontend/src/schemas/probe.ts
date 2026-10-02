import { z } from 'zod';

// An http or https URL that carries no credentials, or null.
function plainHttpUrl(value: string): URL | null {
  try {
    const url = new URL(value);
    const http = url.protocol === 'http:' || url.protocol === 'https:';
    return http && !url.username && !url.password ? url : null;
  } catch {
    return null;
  }
}

// The server checks this again with its own parser; here it only has to turn
// away what is plainly not a bare loopback origin, and nothing the server takes.
function isLoopbackOrigin(value: string): boolean {
  if (value === '') return true;
  const url = plainHttpUrl(value);
  if (!url || url.search || url.hash || url.pathname !== '/') return false;
  return url.hostname === '[::1]' || /^127(\.\d{1,3}){3}$/.test(url.hostname);
}

function isPublicPageUrl(value: string): boolean {
  return value === '' || plainHttpUrl(value) !== null;
}

export const ProbeSettingsFormSchema = z.object({
  url: z.string().trim().refine(isLoopbackOrigin, 'pages.probe.errLiteUrl'),
  publicUrl: z.string().trim().refine(isPublicPageUrl, 'pages.probe.errPublicUrl'),
});
export type ProbeSettingsFormValues = z.infer<typeof ProbeSettingsFormSchema>;
