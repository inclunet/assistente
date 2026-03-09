export type MonacoPosition = {
  lineNumber: number;
  column: number;
};

export type MonacoInsertHeuristicInput = {
  hasFocus: boolean;
  selectionIsEmpty: boolean;
  selectionStart: MonacoPosition;
  currentText: string;
  content: string;
};

export type MonacoInsertHeuristicResult = {
  useSelection: boolean;
  separator: string;
  textToInsert: string;
};

export function computeMonacoInsertText(input: MonacoInsertHeuristicInput): MonacoInsertHeuristicResult {
  const sel = input.selectionStart;
  const isDefaultTopLeft = sel.lineNumber === 1 && sel.column === 1;

  const useSelection = !input.selectionIsEmpty || input.hasFocus || !isDefaultTopLeft;

  const currentText = String(input.currentText ?? '');
  const separator = useSelection
    ? ''
    : (currentText.trim().length > 0
        ? (currentText.endsWith('\n\n') ? '' : '\n\n')
        : '');

  const textToInsert = separator + String(input.content ?? '');

  return { useSelection, separator, textToInsert };
}
