/**
 * Fila de salvamentos por chave, no escopo do módulo: sobrevive à desmontagem
 * de quem enfileirou. Telas que salvam a cada alteração e podem ser fechadas
 * no meio de um salvamento usam a mesma chave para enfileirar e para esperar
 * a fila esvaziar antes de reler os dados, evitando partir de um estado antigo.
 */
const tails = new Map<string, Promise<unknown>>();

export function enqueueSave<T>(key: string, task: () => Promise<T>): Promise<T> {
  const previous = tails.get(key) ?? Promise.resolve();
  const run = previous.then(task);
  const tail = run.catch(() => undefined);
  tails.set(key, tail);
  void tail.then(() => {
    if (tails.get(key) === tail) tails.delete(key);
  });
  return run;
}

export function whenSavesSettled(key: string): Promise<void> {
  return (tails.get(key) ?? Promise.resolve()).then(() => undefined);
}

export const taskListWorkflowSaveKey = (taskListId: string) => `tasklist:${taskListId}:workflow`;
export const taskListCustomActionsSaveKey = (taskListId: string) => `tasklist:${taskListId}:custom-actions`;
