import type { TFunction } from 'i18next';

/**
 * Classes de ação de um agente de código (backend: `acp.ToolKind`).
 *
 * O conjunto mora aqui porque quem o consulta está em lugares diferentes — o
 * aviso na conversa e a tela de autorizações —, e cada um nomeia a mesma classe
 * de um jeito: um dentro de uma frase ("pediu permissão para executar um
 * comando"), o outro como item de lista ("Executar comandos"). Duas listas de
 * códigos se afastariam, e a que ficasse para trás mostraria código cru na tela.
 */
const ACTION_KEYS: Record<string, string> = {
  read: 'read',
  edit: 'edit',
  delete: 'delete',
  move: 'move',
  search: 'search',
  execute: 'execute',
  think: 'think',
  fetch: 'fetch',
  switch_mode: 'switchMode',
  other: 'other',
};

/**
 * Sufixo da chave de tradução da classe. O que a interface não conhece cai em
 * `unknown`: um código cru no meio da frase não diz nada a quem lê.
 */
export function agentActionKey(action?: string): string {
  return (action && ACTION_KEYS[action]) || 'unknown';
}

/**
 * Nome da classe como item de lista ("Executar comandos"). É o mesmo texto da
 * tela de autorizações de propósito: o aviso na conversa diz que existe algo a
 * revogar, e quem for revogar precisa reconhecer ali a linha pelo nome que
 * acabou de ler.
 */
export function agentActionClassName(t: TFunction, action?: string): string {
  return t(`agentPermissions.action.${agentActionKey(action)}`);
}
