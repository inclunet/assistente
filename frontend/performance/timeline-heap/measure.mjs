import { chromium } from '@playwright/test';
import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import net from 'node:net';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const build = path.join(root, 'build/bin/agent-build');
for (const name of ['TEMP', 'TMP', 'TMPDIR']) {
  const value = path.resolve(process.env[name] ?? '');
  if (!value.startsWith(build + path.sep)) throw new Error(`${name} deve apontar para ${build}`);
}
// Porta local efêmera, sem modificar configuração global do WebView.
const server = net.createServer();
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const port = server.address().port;
await new Promise(resolve => server.close(resolve));
const profile = path.join(build, `webview-heap-${Date.now()}`);
await mkdir(profile, { recursive: true });
const child = spawn(path.join(build, 'timeline-heap.exe'), [path.join(build, 'timeline-heap-assets'), profile], {
  windowsHide: true,
  env: { ...process.env, TIMELINE_HEAP_CDP_PORT: String(port) },
  stdio: ['ignore', 'pipe', 'pipe'],
});
let output = '';
child.stdout.on('data', value => { output += value; });
child.stderr.on('data', value => { output += value; });
let launchError;
child.on('error', error => { launchError = error; });
let browser;
let measurementError;
try {
  const deadline = Date.now() + 120_000;
  let connectionError;
  while (!browser && Date.now() < deadline) {
    if (launchError) throw launchError;
    if (child.exitCode !== null) throw new Error(`WebView encerrou: ${output}`);
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`, { signal: AbortSignal.timeout(1000) });
      if (!response.ok) throw new Error(`CDP HTTP ${response.status}`);
      browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`, { timeout: 30_000 });
    } catch (error) { connectionError = error; await new Promise(resolve => setTimeout(resolve, 500)); }
  }
  if (!browser) throw new Error(`WebView indisponível: ${output}`, { cause: connectionError });
  console.log(`WebView conectada: ${await browser.version()}`);
  const context = browser.contexts()[0];
  let page = context.pages()[0];
  if (!page) page = await context.waitForEvent('page', { timeout: 30_000 });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.waitForFunction(() => !!window.timelineHeap, { timeout: 30_000 });
  const cdp = await context.newCDPSession(page);
  const samples = [];
  async function sample(messages, cycle, stage) {
    await cdp.send('HeapProfiler.collectGarbage');
    const heap = await cdp.send('Runtime.getHeapUsage');
    const instances = {};
    let cachedItems = 0;
    try {
      for (const name of ['client', 'presentation', 'conversation']) {
        const prototype = await cdp.send('Runtime.evaluate', {
          expression: `window.timelineHeap.prototypes.${name}`, objectGroup: 'heap-probe',
        });
        const objects = await cdp.send('Runtime.queryObjects', {
          prototypeObjectId: prototype.result.objectId, objectGroup: 'heap-probe',
        });
        const length = await cdp.send('Runtime.callFunctionOn', {
          objectId: objects.objects.objectId,
          functionDeclaration: 'function() { return { count: this.length, cachedItems: this.reduce((total, value) => total + (value.cachedItems ?? 0), 0) }; }',
          returnByValue: true,
        });
        instances[name] = length.result.value.count;
        if (name === 'conversation') cachedItems = length.result.value.cachedItems;
      }
    } finally { await cdp.send('Runtime.releaseObjectGroup', { objectGroup: 'heap-probe' }); }
    samples.push({ messages, cycle, stage, ...heap, instances, cachedItems });
    console.log(JSON.stringify(samples.at(-1)));
    if (cachedItems > 240 || (stage === 'logout' && cachedItems !== 0)) throw new Error('Cache excedeu o contrato após fechamento/logout');
    if (instances.presentation !== 0 || (stage === 'logout' && (instances.client !== 0 || instances.conversation !== 0))) {
      throw new Error('Instâncias da timeline permaneceram retidas após encerramento');
    }
  }
  for (const messages of [100, 500, 1000]) {
    await page.evaluate(() => window.timelineHeap.logout());
    // Aquecimento cobre imports, compilação JIT e primeira montagem real.
    for (let cycle = 0; cycle < 5; cycle++) await page.evaluate(n => window.timelineHeap.cycle(n), messages);
    await sample(messages, 0, 'warm');
    for (let cycle = 1; cycle <= 30; cycle++) {
      await page.evaluate(n => window.timelineHeap.cycle(n), messages);
      if (cycle % 5 === 0) await sample(messages, cycle, 'closed');
    }
    await page.evaluate(() => window.timelineHeap.logout());
    await sample(messages, 30, 'logout');
  }
  if (errors.length) throw new Error(errors.join('\n'));
  const result = { runtime: await browser.version(), capturedAt: new Date().toISOString(), samples };
  await writeFile(path.join(build, 'results/timeline-webview-heap.json'), JSON.stringify(result, null, 2));
} catch (error) {
  measurementError = error;
  throw error;
} finally {
  const cleanupErrors = [];
  try { await browser?.close(); } catch (error) { cleanupErrors.push(error); }
  try {
    if (child.pid && child.exitCode === null && child.signalCode === null) {
      await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('Host WebView não encerrou')), 5000);
        child.once('exit', () => { clearTimeout(timeout); resolve(); });
        if (!child.kill()) { clearTimeout(timeout); reject(new Error('Falha ao encerrar host WebView')); }
      });
    }
  } catch (error) { cleanupErrors.push(error); }
  if (cleanupErrors.length) {
    if (!measurementError) throw new AggregateError(cleanupErrors, 'Limpeza do harness falhou');
    console.error('Falha adicional na limpeza:', cleanupErrors);
  }
}
