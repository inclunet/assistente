import { Component, type ErrorInfo, type ReactNode } from 'react';
import { logger } from '../../utils/logger';
import { AppErrorScreen } from './AppErrorScreen';

interface AppErrorBoundaryProps {
  children: ReactNode;
}

interface AppErrorBoundaryState {
  hasError: boolean;
  error: unknown;
  componentStack: string | null;
}

/**
 * Boundary da árvore do app.
 *
 * Fica dentro do elemento da rota, e não em volta do `RouterProvider`, porque o
 * boundary interno do react-router capturaria o erro antes — e ele guarda só o
 * erro, sem a árvore de componentes. Num laço de render, por exemplo, o stack
 * do erro é todo interno do React: sem `componentStack` não há como saber que
 * componente entrou em laço.
 */
export class AppErrorBoundary extends Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  state: AppErrorBoundaryState = { hasError: false, error: null, componentStack: null };

  static getDerivedStateFromError(error: unknown): Partial<AppErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    this.setState({ componentStack: info.componentStack ?? null });
    logger.error('[App] erro não tratado na renderização', error, info.componentStack);
  }

  render(): ReactNode {
    if (this.state.hasError) {
      return <AppErrorScreen error={this.state.error} componentStack={this.state.componentStack} />;
    }
    return this.props.children;
  }
}
