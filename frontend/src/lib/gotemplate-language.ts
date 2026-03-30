import type * as Monaco from 'monaco-editor';

export const GOTEMPLATE_LANGUAGE_ID = 'gotemplate';

export const GOTEMPLATE_FUNCTIONS = [
  { name: 'pluck', detail: 'pluck .list "field"', doc: 'Extrai um campo de cada item de uma slice' },
  { name: 'any', detail: 'any .list "path" "value"', doc: 'Verifica se algum item tem campo == valor' },
  { name: 'date', detail: 'date .time "2006-01-02"', doc: 'Formata time com layout Go' },
  { name: 'now', detail: 'now', doc: 'Retorna time.Now()' },
  { name: 'join', detail: 'join .list ", "', doc: 'Concatena slice com separador' },
  { name: 'secret', detail: 'secret "key"', doc: 'Busca secret pelo nome' },
  { name: 'json', detail: 'json .value', doc: 'Serializa valor para JSON string' },
  { name: 'default', detail: 'default fallback .value', doc: 'Retorna fallback se valor for zero/vazio' },
] as const;

export const GOTEMPLATE_KEYWORDS = [
  'if', 'else', 'end', 'range', 'with', 'define', 'template', 'block', 'nil', 'true', 'false',
];

export const monarchLanguage: Monaco.languages.IMonarchLanguage = {
  defaultToken: '',
  tokenPostfix: '.gotemplate',

  keywords: GOTEMPLATE_KEYWORDS,
  functions: GOTEMPLATE_FUNCTIONS.map((f) => f.name),

  tokenizer: {
    root: [
      [/\{\{-?\s*\/\*/, 'comment', '@comment'],
      [/\{\{-?/, { token: 'delimiter.bracket', next: '@template' }],
      [/./, 'string'],
    ],

    comment: [
      [/\*\/\s*-?\}\}/, 'comment', '@pop'],
      [/./, 'comment'],
    ],

    template: [
      [/-?\}\}/, { token: 'delimiter.bracket', next: '@pop' }],
      [/\s+/, ''],

      [/\|/, 'delimiter.pipe'],

      [/\$[a-zA-Z_]\w*/, 'variable'],

      [/(\.)([a-zA-Z_]\w*)/, ['delimiter', 'variable.field']],
      [/\./, 'delimiter'],

      [/"[^"]*"/, 'string.template'],
      [/`[^`]*`/, 'string.template'],

      [/\d+(\.\d+)?/, 'number'],

      [
        /[a-zA-Z_]\w*/,
        {
          cases: {
            '@keywords': 'keyword',
            '@functions': 'predefined',
            '@default': 'identifier',
          },
        },
      ],

      [/:=/, 'operator'],
      [/[()=]/, 'delimiter.parenthesis'],
    ],
  },
};

export const languageConfiguration: Monaco.languages.LanguageConfiguration = {
  comments: {
    blockComment: ['{{/*', '*/}}'],
  },
  brackets: [['{{', '}}']],
  autoClosingPairs: [
    { open: '{{', close: '}}' },
    { open: '"', close: '"' },
    { open: '`', close: '`' },
    { open: '(', close: ')' },
  ],
  surroundingPairs: [
    { open: '{{', close: '}}' },
    { open: '"', close: '"' },
    { open: '`', close: '`' },
  ],
};

let registered = false;

export function registerGoTemplateLanguage(monaco: typeof Monaco): void {
  if (registered) return;
  registered = true;

  monaco.languages.register({ id: GOTEMPLATE_LANGUAGE_ID });
  monaco.languages.setMonarchTokensProvider(GOTEMPLATE_LANGUAGE_ID, monarchLanguage);
  monaco.languages.setLanguageConfiguration(GOTEMPLATE_LANGUAGE_ID, languageConfiguration);
}
