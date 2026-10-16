/** Ações duráveis; argumentos e origem são resolvidos pelo backend. */
export function isCommandLayerAction(id: string): boolean {
  return id === 'layer.activate' || id === 'layer.toggle' || id === 'layer.back';
}
