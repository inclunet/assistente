import type { TFunction } from 'i18next';

/**
 * Texto de diálogo vindo do backend (AEP-0085).
 *
 * O backend manda a chave de tradução com os valores que ela interpola e o
 * texto pronto em pt-BR. A string solta é a forma curta de "só texto": é o que
 * chega de quem ainda não migrou para chaves e de tudo que não se traduz
 * (conteúdo do modelo, rótulo que o agente ofereceu, nome de modelo).
 */
export type QuestionnaireText =
  | string
  | {
      key?: string;
      params?: Record<string, unknown>;
      fallback?: string;
    };

/**
 * Texto que vai para a tela: traduz quando há chave e cai no texto pronto do
 * backend quando não há tradução para ela.
 *
 * Nunca devolve vazio por chave faltando — o diálogo é lido por leitor de telas,
 * e um título ou botão em branco tira de quem lê a informação necessária para
 * decidir. Sem chave e sem fallback, sobra o `defaultValue` de quem chamou.
 */
export function resolveQuestionnaireText(
  t: TFunction,
  value: QuestionnaireText | undefined | null,
  defaultValue = ''
): string {
  if (value === undefined || value === null) return defaultValue;
  if (typeof value === 'string') return value || defaultValue;

  const fallback = value.fallback ?? '';
  const key = value.key ?? '';
  if (!key) return fallback || defaultValue;

  // `replace` isola os parâmetros do backend das opções do i18next: um params
  // chamado `count` ou `lng` mudaria o comportamento da tradução se fosse
  // espalhado nas opções. O fallback já vem interpolado pelo backend.
  return t(key, {
    replace: value.params ?? {},
    defaultValue: fallback || defaultValue || key,
  });
}

/**
 * Valor estável de uma opção de escolha: o que volta ao backend em `answers`.
 *
 * É o texto pronto (fallback), nunca a tradução — o backend reencontra a opção
 * pelo que ele mesmo mandou, e traduzir o valor faria a resposta não bater com
 * nenhuma opção oferecida (escopo de autorização de rede, decisão de permissão
 * do agente, provedor escolhido no wizard).
 */
export function questionnaireOptionValue(value: QuestionnaireText): string {
  if (typeof value === 'string') return value;
  return value.fallback || value.key || '';
}
