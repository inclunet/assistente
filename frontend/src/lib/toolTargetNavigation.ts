export type ToolNavigationTarget =
  | { kind: 'url'; url: string }
  | { kind: 'file'; path: string };

export async function openToolNavigationTarget(target: ToolNavigationTarget): Promise<void> {
  if (target.kind === 'url') {
    const { BrowserOpenURL } = await import('@wailsjs/runtime/runtime');
    await BrowserOpenURL(target.url);
    return;
  }

  const { executeDeepLink } = await import('./deepLinks');
  await executeDeepLink(
    { type: 'tab:new', tabType: 'editor', file: target.path },
    { navigate: () => undefined },
  );
}
