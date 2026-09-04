import { parseToolSource } from './toolSource';

export type ToolPolicyState = 'disabled' | 'on_demand' | 'preloaded';

export interface ToolPolicyTarget {
  name: string;
  package?: string;
  optIn?: boolean;
}

type SelectorKind = 'literal' | 'native-all' | 'mcp-all' | 'package-all' | 'mcp-server' | 'package';

interface Selector {
  canonical: string;
  kind: SelectorKind;
  value?: string;
  specificity: number;
}

export interface ToolPolicyMatch {
  state: ToolPolicyState;
  selector?: string;
  specificity: number;
  explicit: boolean;
  literal: boolean;
  deniedOptIn?: boolean;
}

function delimited(value: string, prefix: string, suffix: string): string | null {
  if (!value.startsWith(prefix) || !value.endsWith(suffix)) return null;
  const middle = value.slice(prefix.length, value.length - suffix.length).trim();
  return middle !== '' && middle !== '*' ? middle : null;
}

function scopedSelector(kind: 'mcp-server' | 'package', value: string): Selector | null {
  if (value === '' || /[/*:]/.test(value)) return null;
  return {
    canonical: kind === 'mcp-server' ? `mcp/${value}/*` : `package/${value}/*`,
    kind,
    value,
    specificity: 2,
  };
}

export function parseToolPolicySelector(raw: string): Selector | null {
  const value = raw.trim();
  if (value === '') return null;
  if (value === '*') return { canonical: '*', kind: 'native-all', specificity: 1 };
  if (value === 'mcp/*' || value === 'mcp_*__*') {
    return { canonical: 'mcp/*', kind: 'mcp-all', specificity: 1 };
  }
  if (value === 'package/*') {
    return { canonical: 'package/*', kind: 'package-all', specificity: 1 };
  }

  const mcpSlash = delimited(value, 'mcp/', '/*');
  if (mcpSlash) return scopedSelector('mcp-server', mcpSlash);
  const mcpColon = delimited(value, 'mcp:', '/*');
  if (mcpColon) return scopedSelector('mcp-server', mcpColon);
  const historicalMcp = delimited(value, 'mcp__', '__*');
  if (historicalMcp) return scopedSelector('mcp-server', historicalMcp);
  const canonicalMcp = delimited(value, 'mcp_', '__*');
  if (canonicalMcp) return scopedSelector('mcp-server', canonicalMcp);
  const packageSlash = delimited(value, 'package/', '/*');
  if (packageSlash) return scopedSelector('package', packageSlash);
  const shortPackage = delimited(value, '', '/*');
  if (shortPackage && !shortPackage.includes('/')) return scopedSelector('package', shortPackage);

  return { canonical: value, kind: 'literal', value, specificity: 3 };
}

function selectorMatches(selector: Selector, target: ToolPolicyTarget): boolean {
  const source = parseToolSource(target.name);
  switch (selector.kind) {
    case 'literal': return target.name === selector.value;
    case 'native-all': return source.type !== 'mcp';
    case 'mcp-all': return source.type === 'mcp';
    case 'package-all': return (target.package?.trim() ?? '') !== '';
    case 'mcp-server': return source.type === 'mcp' && source.serverSlug === selector.value;
    case 'package': return target.package === selector.value;
  }
}

export function normalizeToolPolicyState(state: string): ToolPolicyState {
  const normalized = state.trim();
  if (normalized === 'on_demand' || normalized === 'preloaded') return normalized;
  return 'disabled';
}

function stateRank(state: ToolPolicyState): number {
  if (state === 'disabled') return 0;
  if (state === 'on_demand') return 1;
  return 2;
}

export function normalizeToolPolicyMap(policy: Record<string, string> | null): Record<string, string> {
  const normalized: Record<string, string> = {};
  for (const [raw, state] of Object.entries(policy ?? {})) {
    const selector = parseToolPolicySelector(raw);
    if (!selector) continue;
    const existing = normalized[selector.canonical];
    if (existing != null
      && stateRank(normalizeToolPolicyState(existing)) <= stateRank(normalizeToolPolicyState(state))) {
      continue;
    }
    normalized[selector.canonical] = state;
  }
  return normalized;
}

export function resolveToolPolicy(
  policy: Record<string, string>,
  defaultState: string | null | undefined,
  target: ToolPolicyTarget,
): ToolPolicyMatch {
  const fallback = defaultState?.trim() === 'on_demand' ? 'on_demand' : 'disabled';
  let best: ToolPolicyMatch = {
    state: fallback,
    specificity: 0,
    explicit: false,
    literal: false,
  };
  for (const [raw, rawState] of Object.entries(policy)) {
    const selector = parseToolPolicySelector(raw);
    if (!selector || !selectorMatches(selector, target)) continue;
    const state = normalizeToolPolicyState(rawState);
    if (best.explicit && selector.specificity < best.specificity) continue;
    if (best.explicit && selector.specificity === best.specificity
      && stateRank(best.state) <= stateRank(state)) continue;
    best = {
      state,
      selector: selector.canonical,
      specificity: selector.specificity,
      explicit: true,
      literal: selector.kind === 'literal',
    };
  }
  if (target.optIn && best.state !== 'disabled' && !best.literal) {
    return { ...best, state: 'disabled', deniedOptIn: true };
  }
  return best;
}
