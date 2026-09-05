import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { build } from 'esbuild';
import { renderToString } from 'react-dom/server';

test('the main app imports and renders without a Tauri bridge, preserving native window routing', async () => {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const { outputFiles } = await build({
    absWorkingDir: root,
    stdin: {
      contents: 'export { default } from "./src/App"; export { initI18n } from "./src/i18n";',
      resolveDir: root,
    },
    bundle: true,
    write: false,
    format: 'cjs',
    platform: 'browser',
    external: ['react', 'react-dom', 'react-dom/*'],
    loader: { '.css': 'empty', '.module.css': 'empty', '.svg': 'dataurl', '.png': 'dataurl' },
    define: { 'import.meta.env': '{}' },
    logLevel: 'silent',
  });
  const appModule = { exports: {} };
  const events = new EventTarget();
  const storage = new Map();
  const browser = {
    console,
    module: appModule,
    exports: appModule.exports,
    require: createRequire(new URL('../package.json', import.meta.url)),
    setTimeout: () => 1,
    clearTimeout: () => {},
    addEventListener: events.addEventListener.bind(events),
    removeEventListener: events.removeEventListener.bind(events),
    localStorage: {
      getItem: (key) => storage.get(key) ?? null,
      setItem: (key, value) => storage.set(key, String(value)),
      removeItem: (key) => storage.delete(key),
    },
    URL,
    TextEncoder,
    TextDecoder,
    navigator: { platform: 'MacIntel', userAgent: 'Mozilla/5.0', language: 'en-US' },
  };
  // Deliberately omit __TAURI_INTERNALS__: this is an ordinary browser window.
  browser.window = browser;
  const evaluate = vm.runInNewContext(
    `(function(require, module, exports) {\n${outputFiles[0].text}\n})`,
    browser,
    { filename: 'App.bundle.cjs' },
  );
  evaluate(browser.require, appModule, appModule.exports);
  await appModule.exports.initI18n();
  const app = appModule.exports.default();
  assert.equal(app.type.name, 'MainApp');
  const markup = renderToString(app);
  assert.match(markup, /class="app-container/);

  browser.isTauri = true;
  for (const label of ['main', 'floating-card', 'instance-floating-card-test']) {
    browser.__TAURI_INTERNALS__ = { metadata: { currentWindow: { label } } };
    const nativeApp = appModule.exports.default();
    assert.equal(nativeApp.type.name, label === 'main' ? 'MainApp' : 'FloatingCardWindow');
  }
});
