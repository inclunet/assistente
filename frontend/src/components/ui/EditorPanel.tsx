import type { ReactNode } from 'react';
import './EditorPanel.css';

export interface EditorPanelProps {
  className?: string;
  children: ReactNode;
}

export function EditorPanel({ className, children }: EditorPanelProps) {
  return <div className={`editor-panel${className ? ` ${className}` : ''}`}>{children}</div>;
}

export interface EditorPanelFieldsProps {
  className?: string;
  children: ReactNode;
}

export function EditorPanelFields({ className, children }: EditorPanelFieldsProps) {
  return <div className={`editor-panel__fields${className ? ` ${className}` : ''}`}>{children}</div>;
}

export interface EditorPanelFooterProps {
  className?: string;
  children: ReactNode;
}

export function EditorPanelFooter({ className, children }: EditorPanelFooterProps) {
  return <div className={`editor-panel__footer${className ? ` ${className}` : ''}`}>{children}</div>;
}
