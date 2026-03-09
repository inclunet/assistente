export type RichTextChain = {
  focus: (position?: any) => RichTextChain;
  setTextSelection: (range: { from: number; to: number }) => RichTextChain;
  insertContent: (content: any) => RichTextChain;
  run: () => any;
};

export type RichTextEditorLike = {
  isEditable?: boolean;
  setEditable?: (editable: boolean) => void;
  chain: () => RichTextChain;
};

export function applyRichTextInsert(params: {
  rich: RichTextEditorLike;
  from: number;
  to: number;
  contentToInsert: any;
}): void {
  const rich = params.rich;
  const from = Number(params.from);
  const to = Number(params.to);

  const wasEditable = !!rich.isEditable;
  try {
    if (!wasEditable) rich.setEditable?.(true);
    rich.chain().focus().setTextSelection({ from, to }).insertContent(params.contentToInsert).run();
  } finally {
    if (!wasEditable) rich.setEditable?.(false);
  }
}

export function applyRichTextInsertAtEnd(params: { rich: RichTextEditorLike; contentToInsert: any }): void {
  const rich = params.rich;

  const wasEditable = !!rich.isEditable;
  try {
    if (!wasEditable) rich.setEditable?.(true);
    const chain: any = rich.chain();

    // TipTap suporta focus('end'). Se não suportar, cai para focus() normal.
    let focused: any;
    try {
      focused = chain.focus('end');
    } catch {
      focused = chain.focus();
    }

    focused.insertContent(params.contentToInsert).run();
  } finally {
    if (!wasEditable) rich.setEditable?.(false);
  }
}
