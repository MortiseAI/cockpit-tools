const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { unifyDataDir } = require('./unify-data-dir.cjs');

function fixture(t) {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'cockpit-shared-data-'));
  t.after(() => fs.rmSync(home, { recursive: true, force: true }));
  const prod = path.join(home, '.antigravity_cockpit');
  const dev = path.join(home, '.antigravity_cockpit_dev');
  for (const [dir, value] of [[prod, 'prod'], [dev, 'dev']]) {
    fs.mkdirSync(dir);
    fs.writeFileSync(path.join(dir, 'secure-account-storage.key'), `${value}-fixture-key`);
    fs.writeFileSync(path.join(dir, 'account.json'), `${value}-fixture-ciphertext`);
  }
  return { home, prod, dev };
}

test('preview leaves both stores unchanged', t => {
  const { home, prod, dev } = fixture(t);
  assert.equal(unifyDataDir({ home }).status, 'preview');
  assert.notEqual(fs.realpathSync(prod), fs.realpathSync(dev));
  assert.equal(fs.readdirSync(home).length, 2);
});

for (const source of ['dev', 'prod']) {
  test(`shares the ${source} store and backs up the other encrypted store intact`, t => {
    const { home, prod, dev } = fixture(t);
    const selected = source === 'dev' ? dev : prod;
    const other = source === 'dev' ? 'prod' : 'dev';
    const result = unifyDataDir({ home, source, apply: true });
    assert.equal(result.status, 'unified');
    assert.equal(fs.realpathSync(prod), fs.realpathSync(dev));
    assert.equal(fs.lstatSync(selected).isSymbolicLink(), false);
    assert.equal(fs.readFileSync(path.join(prod, 'secure-account-storage.key'), 'utf8'), `${source}-fixture-key`);
    assert.equal(fs.readFileSync(path.join(result.backup, 'secure-account-storage.key'), 'utf8'), `${other}-fixture-key`);
    assert.equal(fs.readFileSync(path.join(result.backup, 'account.json'), 'utf8'), `${other}-fixture-ciphertext`);
    fs.writeFileSync(path.join(dev, 'quota.json'), '54');
    assert.equal(fs.readFileSync(path.join(prod, 'quota.json'), 'utf8'), '54');
    assert.equal(unifyDataDir({ home, source, apply: true }).status, 'already-unified');
    assert.equal(unifyDataDir({ home, source: other, apply: true }).status, 'already-unified');
  });
}

test('the selected store may remain open but the replaced store must be stopped', t => {
  const { home, prod, dev } = fixture(t);
  fs.writeFileSync(path.join(prod, 'server.json'), JSON.stringify({ pid: process.pid }));
  assert.throws(() => unifyDataDir({ home, apply: true }), /Quit the Cockpit instance/);
  assert.equal(fs.readFileSync(path.join(prod, 'account.json'), 'utf8'), 'prod-fixture-ciphertext');
  fs.unlinkSync(path.join(prod, 'server.json'));
  fs.writeFileSync(path.join(dev, 'server.json'), JSON.stringify({ pid: process.pid }));
  assert.equal(unifyDataDir({ home, apply: true }).status, 'unified');
});

test('supports a dev-only installation without creating a second store', t => {
  const { home, prod, dev } = fixture(t);
  fs.rmSync(prod, { recursive: true });
  const result = unifyDataDir({ home, apply: true });
  assert.equal(result.backup, null);
  assert.equal(fs.realpathSync(prod), fs.realpathSync(dev));
});

test('does not create empty accounts when the selected source is missing', t => {
  const { home, dev } = fixture(t);
  fs.rmSync(dev, { recursive: true });
  assert.throws(() => unifyDataDir({ home, apply: true }), /Source directory does not exist/);
});

test('restores the original directory when creating the compatibility link fails', t => {
  const { home, prod, dev } = fixture(t);
  t.mock.method(fs, 'symlinkSync', () => { throw new Error('fixture link failure'); });
  assert.throws(() => unifyDataDir({ home, apply: true }), /fixture link failure/);
  assert.equal(fs.readFileSync(path.join(prod, 'secure-account-storage.key'), 'utf8'), 'prod-fixture-key');
  assert.equal(fs.readFileSync(path.join(dev, 'secure-account-storage.key'), 'utf8'), 'dev-fixture-key');
  assert.equal(fs.existsSync(path.join(home, '.cockpit-data-dir-migration.lock')), false);
});

test('rejects a source nested inside the store being replaced', t => {
  const { home, prod, dev } = fixture(t);
  fs.rmSync(dev, { recursive: true });
  const nested = path.join(prod, 'nested');
  fs.mkdirSync(nested);
  fs.symlinkSync(nested, dev, process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => unifyDataDir({ home, apply: true }), /inside the directory/);
  assert.equal(fs.readFileSync(path.join(prod, 'account.json'), 'utf8'), 'prod-fixture-ciphertext');
});
