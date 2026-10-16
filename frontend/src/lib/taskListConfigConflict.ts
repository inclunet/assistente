/**
 * O backend recusa gravar workflow/custom actions quando a configuração mudou
 * desde que o editor a leu (database.ErrTaskListConfigConflict). O Wails só
 * entrega o texto do erro, que começa com este código estável.
 */
export const TASK_LIST_CONFIG_CONFLICT_CODE = 'TASKLIST_CONFIG_CONFLICT';

export function isTaskListConfigConflict(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? '');
  return message.trimStart().startsWith(TASK_LIST_CONFIG_CONFLICT_CODE);
}
