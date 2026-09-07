import { useRouteError } from 'react-router-dom';
import { AppErrorScreen } from './AppErrorScreen';

/**
 * Rede de segurança para o que o `AppErrorBoundary` não alcança: erros do
 * próprio roteador (rota inexistente, falha ao carregar um módulo de rota).
 * Aqui não existe árvore de componentes — o react-router não a repassa.
 */
export function RouteErrorScreen() {
  const error = useRouteError();
  return <AppErrorScreen error={error} />;
}
