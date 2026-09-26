import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

const managerPagePath = resolve(process.cwd(), 'src/pages/CommandSettingsPage.tsx');
const forbiddenModifierNames = new Set(['ctrlKey', 'altKey', 'metaKey']);

function shortcutArchitectureViolations(sourceText: string): string[] {
  const source = ts.createSourceFile('fixture.tsx', sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const violations: string[] = [];

  const report = (node: ts.Node, kind: string) => {
    const { line } = source.getLineAndCharacterOfPosition(node.getStart(source));
    violations.push(`${kind}@${line + 1}`);
  };

  const visit = (node: ts.Node) => {
    if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression) &&
        node.expression.name.text === 'addEventListener') {
      const eventName = node.arguments[0];
      if (eventName && (ts.isStringLiteral(eventName) || ts.isNoSubstitutionTemplateLiteral(eventName)) &&
          ['keydown', 'keyup'].includes(eventName.text.toLowerCase())) {
        report(node, `listener:${eventName.text.toLowerCase()}`);
      }
    }

    if (ts.isJsxAttribute(node) && ts.isIdentifier(node.name) &&
        ['onKeyDown', 'onKeyUp'].includes(node.name.text)) {
      report(node, `jsx:${node.name.text}`);
    }

    if (ts.isIdentifier(node) && forbiddenModifierNames.has(node.text)) {
      report(node, `modifier:${node.text}`);
    }

    ts.forEachChild(node, visit);
  };

  visit(source);
  return violations;
}

function objectProperty(object: ts.ObjectLiteralExpression, name: string): ts.ObjectLiteralElementLike | undefined {
  return object.properties.find((property) =>
    (ts.isPropertyAssignment(property) || ts.isShorthandPropertyAssignment(property)) &&
    ts.isIdentifier(property.name) && property.name.text === name);
}

describe('CommandSettingsPage architecture boundary for shortcuts', () => {
  it('detects DOM key listeners and JSX key handlers in TSX fixtures', () => {
    const fixture = `
      target.addEventListener('keydown', onKey);
      target.addEventListener("keyup", onKeyUp);
      const Example = () => <section onKeyDown={onKey} onKeyUp={onKeyUp} />;
    `;

    expect(shortcutArchitectureViolations(fixture)).toEqual([
      'listener:keydown@2',
      'listener:keyup@3',
      'jsx:onKeyDown@4',
      'jsx:onKeyUp@4',
    ]);
  });

  it('detects literal control, alt and meta modifier reads and catches local gesture handlers on this page', () => {
    const shortcutFixture = `
      if (event.ctrlKey && event.key === 'n') openNew();
      if (event.altKey || event.metaKey) return;
    `;
    const nativeGestureFixture = `
      const MenuItem = () => <button onKeyDown={(event) => moveByArrow(event.key)} />;
      target.addEventListener('keydown', (event) => closeOnEscape(event.key));
    `;

    expect(shortcutArchitectureViolations(shortcutFixture)).toEqual([
      'modifier:ctrlKey@2',
      'modifier:altKey@3',
      'modifier:metaKey@3',
    ]);
    expect(shortcutArchitectureViolations(nativeGestureFixture)).toEqual([
      'jsx:onKeyDown@2',
      'listener:keydown@3',
    ]);
  });

  it('keeps settings-manager presentation delegated to its central command id', () => {
    const text = readFileSync(managerPagePath, 'utf8');
    const source = ts.createSourceFile(managerPagePath, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
    let importedCentralID = false;
    let importedHook = false;
    let validHookCall = false;

    for (const statement of source.statements) {
      if (!ts.isImportDeclaration(statement) || !statement.importClause?.namedBindings ||
          !ts.isNamedImports(statement.importClause.namedBindings) ||
          !ts.isStringLiteral(statement.moduleSpecifier) ||
          statement.moduleSpecifier.text !== '../lib/commandPagePresentation') continue;
      for (const specifier of statement.importClause.namedBindings.elements) {
        if (specifier.name.text === 'COMMAND_SETTINGS_CREATE_COMMAND_ID') importedCentralID = true;
        if (specifier.name.text === 'usePagePresentationCommands') importedHook = true;
      }
    }

    const visit = (node: ts.Node) => {
      if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) &&
          node.expression.text === 'usePagePresentationCommands' &&
          node.arguments.length === 1 && ts.isObjectLiteralExpression(node.arguments[0])) {
        const options = node.arguments[0];
        const scope = objectProperty(options, 'settingsManager');
        const commands = objectProperty(options, 'allowedCommands');
        if (scope && ts.isPropertyAssignment(scope) &&
            commands && ts.isPropertyAssignment(commands) && ts.isArrayLiteralExpression(commands.initializer) &&
            commands.initializer.elements.some((element) => ts.isIdentifier(element) &&
              element.text === 'COMMAND_SETTINGS_CREATE_COMMAND_ID')) {
          validHookCall = true;
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(source);

    expect(importedCentralID).toBe(true);
    expect(importedHook).toBe(true);
    expect(validHookCall).toBe(true);
    expect(shortcutArchitectureViolations(text)).toEqual([]);
  });
});
