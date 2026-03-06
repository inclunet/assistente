export type ToastType = 'success' | 'error' | 'warning' | 'info';

export type AddToastFn = (message: string, type: ToastType, duration?: number) => string;

export type FileMenuItem = {
  value: string;
  label: string;
  sublabel?: string;
  disabled?: boolean;
};
