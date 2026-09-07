const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

function directoryExists(dir) {
  try {
    fs.lstatSync(dir);
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
  if (!fs.statSync(dir).isDirectory()) throw new Error(`Not a directory: ${dir}`);
  return true;
}

function assertNotRunning(dir) {
  const file = path.join(dir, 'server.json');
  if (!fs.existsSync(file)) return;
  const { pid } = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (!Number.isInteger(pid) || pid <= 0) return;
  try {
    process.kill(pid, 0);
  } catch (error) {
    if (error.code === 'ESRCH') return;
    throw error;
  }
  throw new Error(`Quit the Cockpit instance using ${dir} before replacing it (PID ${pid}).`);
}

function unifyDataDir({ home = os.homedir(), source = 'dev', apply = false } = {}) {
  if (!['dev', 'prod'].includes(source)) throw new Error('source must be dev or prod');
  home = fs.realpathSync(home);
  const canonical = path.join(home, '.antigravity_cockpit');
  const legacy = path.join(home, '.antigravity_cockpit_dev');
  const selected = source === 'dev' ? legacy : canonical;
  const alias = source === 'dev' ? canonical : legacy;
  if (!directoryExists(selected)) throw new Error(`Source directory does not exist: ${selected}`);
  const hasAlias = directoryExists(alias);
  if (hasAlias && fs.realpathSync(selected) === fs.realpathSync(alias)) {
    return { status: 'already-unified', canonical, storage: fs.realpathSync(selected) };
  }
  // Keep the selected store in place, including encryption keys, open databases
  // and persisted absolute paths. The other name becomes a compatibility alias.
  const storage = fs.realpathSync(selected);
  if (storage === alias || storage.startsWith(alias + path.sep)) {
    throw new Error('The source is inside the directory that would be replaced.');
  }
  if (!apply) return { status: 'preview', canonical, storage, replace: hasAlias ? alias : null };

  const lock = path.join(home, '.cockpit-data-dir-migration.lock');
  const descriptor = fs.openSync(lock, 'wx', 0o600);
  let backup;
  try {
    if (hasAlias) {
      assertNotRunning(alias);
      const backups = path.join(home, '.cockpit-data-dir-backups');
      fs.mkdirSync(backups, { recursive: true, mode: 0o700 });
      const container = fs.mkdtempSync(path.join(backups, 'unify-'));
      backup = path.join(container, path.basename(alias));
      fs.renameSync(alias, backup);
    }
    try {
      fs.symlinkSync(storage, alias, process.platform === 'win32' ? 'junction' : 'dir');
    } catch (error) {
      if (backup) fs.renameSync(backup, alias);
      throw error;
    }
    return { status: 'unified', canonical, storage, backup: backup ?? null };
  } finally {
    fs.closeSync(descriptor);
    fs.unlinkSync(lock);
  }
}

if (require.main === module) {
  const args = process.argv.slice(2);
  if (args.some(arg => !['--apply', '--source=dev', '--source=prod'].includes(arg))) {
    console.error('Usage: node scripts/unify-data-dir.cjs [--source=dev|--source=prod] [--apply]');
    process.exitCode = 1;
  } else {
    try {
      const source = args.includes('--source=prod') ? 'prod' : 'dev';
      console.log(JSON.stringify(unifyDataDir({ source, apply: args.includes('--apply') }), null, 2));
    } catch (error) {
      console.error(error.message);
      process.exitCode = 1;
    }
  }
}

module.exports = { unifyDataDir };
