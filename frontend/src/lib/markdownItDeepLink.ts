import type MarkdownIt from 'markdown-it';
import {
  DEEP_LINK_PREFIX,
  parseDeepLink,
  getDeepLinkTypeClass,
  getDeepLinkLabel,
} from './deepLinks';

/**
 * markdown-it plugin that detects links with the `assistente://` protocol
 * and transforms them into styled deep-link chips with ARIA attributes.
 *
 * Hooks into `link_open` renderer rule to annotate matching `<a>` tokens
 * with CSS classes, data attributes and accessible labels.
 */
export function markdownItDeepLink(md: MarkdownIt): void {
  const defaultLinkOpen =
    md.renderer.rules.link_open ||
    ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));

  md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
    const token = tokens[idx];
    const hrefIndex = token.attrIndex('href');

    if (hrefIndex >= 0) {
      const href = token.attrs![hrefIndex][1];

      if (href.startsWith(DEEP_LINK_PREFIX)) {
        const action = parseDeepLink(href);

        const classIndex = token.attrIndex('class');
        const typeClass = action ? getDeepLinkTypeClass(action) : '';
        const classes = `deep-link ${typeClass}`.trim();

        if (classIndex >= 0) {
          token.attrs![classIndex][1] += ` ${classes}`;
        } else {
          token.attrPush(['class', classes]);
        }

        token.attrPush(['data-deep-link', href]);
        token.attrPush(['role', 'link']);

        if (action) {
          token.attrPush(['aria-label', getDeepLinkLabel(action)]);
        }
      }
    }

    return defaultLinkOpen(tokens, idx, options, env, self);
  };
}
