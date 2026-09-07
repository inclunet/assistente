export interface TelegramForm {
  enabled: boolean;
  botToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

export interface SignalForm {
  enabled: boolean;
  apiURL: string;
  account: string;
  apiToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

export interface SlackForm {
  enabled: boolean;
  botToken: string;
  appToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

export type SignalRegisterStep = 'idle' | 'registering' | 'awaiting_code' | 'verifying' | 'done';

export type SignalConnectionMode = 'register' | 'link';
