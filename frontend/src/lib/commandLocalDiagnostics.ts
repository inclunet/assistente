type Outcome = 'succeeded' | 'cancelled' | 'failed';
type Stage = 'local_ui' | 'creation_selection' | 'creation_catalog';
type Source = 'palette' | 'keyboard.local' | 'ui.action';

/** Diagnóstico oportunista: nunca espera a bridge nem afeta o comando. */
export function beginLocalPaletteDiagnostic(commandID: string, stage: Stage = 'local_ui', source: Source = 'palette'): (outcome: Outcome) => void {
  let invocationID: string;
  try { invocationID = crypto.randomUUID(); } catch { return () => {}; }
  const started = performance.now();
  return (outcome) => {
    try {
      const api = (window as Window & { go?: { app?: { App?: {
        ReportLocalPaletteCommand?: (command: string, id: string, status: Outcome, elapsed: number, stage: Stage, source: Source) => Promise<void>;
      } } } }).go?.app?.App;
      const elapsed = Math.min(60000, Math.max(0, Math.round(performance.now() - started)));
      void api?.ReportLocalPaletteCommand?.(commandID, invocationID, outcome, elapsed, stage, source)?.catch(() => {});
    } catch { /* diagnóstico não interfere no comando */ }
  };
}
