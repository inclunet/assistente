import { describe, expect, it } from 'vitest';
import {
  normalizeToolPolicyMap,
  parseToolPolicySelector,
  resolveToolPolicy,
} from './toolPolicyMatcher';

describe('toolPolicyMatcher', () => {
  it.each([
    ['mcp/atlassian/*', 'mcp/atlassian/*'],
    ['mcp:atlassian/*', 'mcp/atlassian/*'],
    ['mcp_atlassian__*', 'mcp/atlassian/*'],
    ['mcp__atlassian__*', 'mcp/atlassian/*'],
    ['package/history/*', 'package/history/*'],
    ['history/*', 'package/history/*'],
  ])('normaliza o alias %s', (raw, canonical) => {
    expect(parseToolPolicySelector(raw)?.canonical).toBe(canonical);
  });

  it.each([
    'mcp/atlassian cloud/*',
    'mcp:atlassian\tcloud/*',
    'package/my package/*',
    'my package/*',
  ])('rejeita o escopo não canônico %s', (raw) => {
    expect(parseToolPolicySelector(raw)).toBeNull();
  });

  it('aplica literal, wildcard específico, wildcard geral e default nessa ordem', () => {
    const policy = normalizeToolPolicyMap({
      'mcp/*': 'preloaded',
      'mcp/atlassian/*': 'on_demand',
      mcp_atlassian__delete: 'disabled',
    });
    expect(resolveToolPolicy(policy, 'disabled', { name: 'mcp_atlassian__delete' }).state).toBe('disabled');
    expect(resolveToolPolicy(policy, 'disabled', { name: 'mcp_atlassian__search' }).state).toBe('on_demand');
    expect(resolveToolPolicy(policy, 'disabled', { name: 'mcp_slack__send' }).state).toBe('preloaded');
    expect(resolveToolPolicy(policy, 'disabled', { name: 'read_file', package: 'filesystem' }).state).toBe('disabled');
  });

  it('escolhe o estado mais restritivo em empate entre aliases', () => {
    const policy = normalizeToolPolicyMap({
      'mcp/atlassian/*': 'preloaded',
      'mcp:atlassian/*': 'disabled',
      'package/history/*': 'preloaded',
      'history/*': 'on_demand',
    });
    expect(resolveToolPolicy(policy, 'disabled', { name: 'mcp_atlassian__search' }).state).toBe('disabled');
    expect(resolveToolPolicy(policy, 'disabled', {
      name: 'search_conversations',
      package: 'history',
    }).state).toBe('on_demand');
  });

  it('não eleva opt-in por default ou wildcard permissivo', () => {
    const policy = normalizeToolPolicyMap({ '*': 'preloaded', job: 'on_demand' });
    expect(resolveToolPolicy(policy, 'on_demand', {
      name: 'text_edit',
      package: 'filesystem',
      optIn: true,
    }).state).toBe('disabled');
    expect(resolveToolPolicy(policy, 'on_demand', {
      name: 'job',
      package: 'job',
      optIn: true,
    }).state).toBe('on_demand');
  });

  it('aplica package à builtin mcp_server e namespace à MCP canônica', () => {
    const policy = normalizeToolPolicyMap({
      'package/mcp/*': 'preloaded',
      'mcp/atlassian/*': 'on_demand',
    });
    expect(resolveToolPolicy(policy, 'disabled', {
      name: 'mcp_server',
      package: 'mcp',
    }).state).toBe('preloaded');
    expect(resolveToolPolicy(policy, 'disabled', {
      name: 'mcp_atlassian__search',
    }).state).toBe('on_demand');
  });
});
