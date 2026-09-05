const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

function prependPathEntry(currentPath, entry) {
  const entries = String(currentPath || '')
    .split(path.delimiter)
    .filter(Boolean);

  if (!entries.includes(entry)) {
    entries.unshift(entry);
  }

  return entries.join(path.delimiter);
}

function createTauriEnv(overrides = {}) {
  const env = {
    ...process.env,
    ...overrides,
  };

  const cargoHome = env.CARGO_HOME || path.join(os.homedir(), '.cargo');
  const cargoBinPath = path.join(cargoHome, 'bin');
  const cargoExecutable = path.join(
    cargoBinPath,
    process.platform === 'win32' ? 'cargo.exe' : 'cargo',
  );

  // GUI terminals and non-login shells do not always source ~/.cargo/env.
  // Add rustup's default bin directory explicitly so the Tauri CLI can run
  // `cargo metadata` even when Cargo is absent from the inherited PATH.
  if (fs.existsSync(cargoExecutable)) {
    env.PATH = prependPathEntry(env.PATH, cargoBinPath);
  }

  if (process.platform === 'win32') {
    const goBinPath = 'C:\\Program Files\\Go\\bin';
    if (fs.existsSync(goBinPath)) {
      env.PATH = prependPathEntry(env.PATH, goBinPath);
    }
  }

  return env;
}

module.exports = { createTauriEnv };
