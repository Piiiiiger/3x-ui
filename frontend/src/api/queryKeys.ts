export const keys = {
  server: {
    status: () => ['server', 'status'] as const,
    fail2banStatus: () => ['server', 'fail2banStatus'] as const,
  },
  nodes: {
    root: () => ['nodes'] as const,
    list: () => ['nodes', 'list'] as const,
  },
  subBalancers: {
    root: () => ['sub-balancers'] as const,
    list: () => ['sub-balancers', 'list'] as const,
  },
  plans: {
    root: () => ['plans'] as const,
    list: () => ['plans', 'list'] as const,
  },
  ruleTemplates: {
    root: () => ['ruleTemplates'] as const,
    list: () => ['ruleTemplates', 'list'] as const,
    get: (id: number) => ['ruleTemplates', 'get', id] as const,
    versions: (id: number) => ['ruleTemplates', 'versions', id] as const,
    preview: (requestId: number, planId: number) =>
      ['ruleTemplates', 'preview', requestId, planId] as const,
    conversion: (id: number, baseId: number) =>
      ['ruleTemplates', 'conversion', id, baseId] as const,
  },
  traffic: {
    root: () => ['traffic'] as const,
    overview: (period: string) => ['traffic', 'overview', period] as const,
  },
  probe: {
    root: () => ['probe'] as const,
    servers: () => ['probe', 'servers'] as const,
    links: () => ['probe', 'links'] as const,
    settings: () => ['probe', 'settings'] as const,
  },
  settings: {
    root: () => ['settings'] as const,
    all: () => ['settings', 'all'] as const,
    defaults: () => ['settings', 'defaults'] as const,
    factoryDefaults: () => ['settings', 'factoryDefaults'] as const,
  },
  inbounds: {
    root: () => ['inbounds'] as const,
    slim: () => ['inbounds', 'slim'] as const,
    options: () => ['inbounds', 'options'] as const,
  },
  clients: {
    root: () => ['clients'] as const,
    list: (params: unknown) => ['clients', 'list', params] as const,
    all: () => ['clients', 'all'] as const,
    onlines: () => ['clients', 'onlines'] as const,
    onlinesByGuid: () => ['clients', 'onlinesByGuid'] as const,
    activeInbounds: () => ['clients', 'activeInbounds'] as const,
    lastOnline: () => ['clients', 'lastOnline'] as const,
    portal: (email: string) => ['clients', 'portal', email] as const,
  },
  portal: {
    data: (base: string) => ['portal', 'data', base] as const,
    probes: () => ['portal', 'probe'] as const,
    // Keyed by client: one browser can sign in a second person, who must never
    // be handed what the cache holds of the first.
    probe: (base: string, email: string) => ['portal', 'probe', base, email] as const,
  },
  xray: {
    root: () => ['xray'] as const,
    config: () => ['xray', 'config'] as const,
    geodata: {
      root: () => ['xray', 'geodata'] as const,
      files: () => ['xray', 'geodata', 'files'] as const,
      categories: (file: string, query: string) =>
        ['xray', 'geodata', 'categories', file, query] as const,
      entries: (file: string, code: string, query: string, offset: number, limit: number) =>
        ['xray', 'geodata', 'entries', file, code, query, offset, limit] as const,
    },
  },
} as const;
