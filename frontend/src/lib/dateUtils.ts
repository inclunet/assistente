/**
 * Formata timestamp como tempo relativo em português
 */
export function formatRelativeTime(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) {
    return seconds <= 5 ? 'agora' : `há ${seconds}s`;
  } else if (minutes < 2) {
    return 'há 1 min';
  } else if (minutes < 60) {
    return `há ${minutes} min`;
  } else if (hours < 2) {
    return 'há 1 hora';
  } else if (hours < 24) {
    return `há ${hours} horas`;
  } else if (days === 1) {
    return 'ontem';
  } else if (days < 7) {
    return `há ${days} dias`;
  } else if (days < 30) {
    const weeks = Math.floor(days / 7);
    return weeks === 1 ? 'há 1 semana' : `há ${weeks} semanas`;
  } else if (days < 365) {
    const months = Math.floor(days / 30);
    return months === 1 ? 'há 1 mês' : `há ${months} meses`;
  } else {
    const years = Math.floor(days / 365);
    return years === 1 ? 'há 1 ano' : `há ${years} anos`;
  }
}

/**
 * Formata timestamp como tempo relativo LOCALIZADO conforme o idioma atual,
 * usando Intl.RelativeTimeFormat (ex.: "há 5 min" / "5 min ago" / "hace 5 min",
 * "ontem" / "yesterday" / "ayer"). Diferente de formatRelativeTime (strings
 * fixas em pt-BR), respeita o locale — use em UIs multi-idioma.
 *
 * `locale` aceita um tag BCP-47 (ex.: i18n.language: 'pt-BR' | 'en' | 'es').
 */
export function formatRelativeTimeLocalized(timestamp: number, locale: string): string {
  const diffMs = timestamp - Date.now(); // negativo = passado
  const sign = diffMs <= 0 ? -1 : 1;
  const absSeconds = Math.abs(Math.floor(diffMs / 1000));
  const minutes = Math.floor(absSeconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (absSeconds < 60) return rtf.format(sign * absSeconds, 'second');
  if (minutes < 60) return rtf.format(sign * minutes, 'minute');
  if (hours < 24) return rtf.format(sign * hours, 'hour');
  if (days < 7) return rtf.format(sign * days, 'day');
  if (days < 30) return rtf.format(sign * Math.floor(days / 7), 'week');
  if (days < 365) return rtf.format(sign * Math.floor(days / 30), 'month');
  return rtf.format(sign * Math.floor(days / 365), 'year');
}

/**
 * Formata data como string legível
 */
export function formatDate(date: Date | string | number): string {
  const d = typeof date === 'number' ? new Date(date) : typeof date === 'string' ? new Date(date) : date;
  
  return d.toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  });
}

/**
 * Formata data e hora como string legível
 */
export function formatDateTime(date: Date | string | number): string {
  const d = typeof date === 'number' ? new Date(date) : typeof date === 'string' ? new Date(date) : date;
  
  return d.toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
