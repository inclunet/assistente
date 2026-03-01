export type RichTextChain = {
  focus: () => RichTextChain;
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
