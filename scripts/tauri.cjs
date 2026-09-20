const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createTauriEnv } = require('./tauri-env.cjs');

const repoRoot = path.resolve(__dirname, '..');
const goBinPath = 'C:\\Program Files\\Go\\bin';
const rawTauriArgs = process.argv.slice(2);
const isDevCommand = rawTauriArgs[0] === 'dev';
const hasExplicitConfig = rawTauriArgs.some(
  (arg) => arg === '--config' || arg.startsWith('--config='),
);
const tauriArgs =
  isDevCommand && !hasExplicitConfig
    ? ['dev', '--config', 'src-tauri/tauri.dev.conf.json', ...rawTauriArgs.slice(1)]
    : rawTauriArgs;
const commandEnv = isDevCommand
  ? {
      COCKPIT_TOOLS_PROFILE: process.env.COCKPIT_TOOLS_PROFILE || 'dev',
      VITE_COCKPIT_TOOLS_PROFILE: process.env.VITE_COCKPIT_TOOLS_PROFILE || 'dev',
    }
  : {};

const cargoBinPath = path.join(os.homedir(), '.cargo', 'bin');

function withTauriEnv(options = {}) {
  const currentPath = process.env.PATH || '';
  const extraPaths = [cargoBinPath, goBinPath].filter((dir) => fs.existsSync(dir));
  const mergedOptions = extraPaths.length
    ? {
        ...options,
        env: {
          ...options.env,
          PATH: `${extraPaths.join(path.delimiter)}${path.delimiter}${currentPath}`,
        },
      }
    : options;

  return {
    ...mergedOptions,
    env: createTauriEnv({
      ...commandEnv,
      ...mergedOptions.env,
    }),
  };
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: repoRoot,
    stdio: 'inherit',
    shell: false,
    ...withTauriEnv(options),
  });

  if (result.error) {
    throw result.error;
  }

  if (result.status !== 0) {
    process.exit(typeof result.status === 'number' ? result.status : 1);
  }
}

function runFinal(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: repoRoot,
    stdio: 'inherit',
    shell: false,
    ...withTauriEnv(options),
  });

  if (result.error) {
    throw result.error;
  }

  process.exit(typeof result.status === 'number' ? result.status : 1);
}

function runTauriDirect() {
  run('npm.cmd', ['run', 'sync-version'], { shell: process.platform === 'win32' });
  runFinal('npx.cmd', ['tauri', ...tauriArgs], { shell: process.platform === 'win32' });
}

if (process.platform !== 'win32') {
  run('npm', ['run', 'sync-version']);
  runFinal('npx', ['tauri', ...tauriArgs]);
}

const vcvarsCandidates = [
  'C:\\Program Files\\Microsoft Visual Studio\\2022\\Professional\\VC\\Auxiliary\\Build\\vcvars64.bat',
  'C:\\Program Files\\Microsoft Visual Studio\\2022\\Enterprise\\VC\\Auxiliary\\Build\\vcvars64.bat',
  'C:\\Program Files\\Microsoft Visual Studio\\2022\\Community\\VC\\Auxiliary\\Build\\vcvars64.bat',
  'C:\\Program Files (x86)\\Microsoft Visual Studio\\2022\\BuildTools\\VC\\Auxiliary\\Build\\vcvars64.bat',
];
const vcvars64Path = vcvarsCandidates.find((candidate) => fs.existsSync(candidate)) || '';

if (!vcvars64Path) {
  console.warn('vcvars64.bat not found, falling back to the existing shell environment.');
  runTauriDirect();
}

const tempScriptPath = path.join(os.tmpdir(), `cockpit-tools-tauri-${process.pid}.cmd`);
const tauriCliPath = path.join(repoRoot, 'node_modules', '.bin', 'tauri.cmd');
if (!fs.existsSync(tauriCliPath)) {
  console.warn('Local tauri CLI not found, falling back to the existing shell environment.');
  runTauriDirect();
}

const quotedArgs = tauriArgs.map((arg) => {
  if (/[\s"]/u.test(arg)) {
    return `"${arg.replace(/"/g, '""')}"`;
  }
  return arg;
});
const scriptBody = [
  '@echo off',
  `set "PATH=${cargoBinPath};${goBinPath};%PATH%"`,
  `call "${vcvars64Path}"`,
  'if errorlevel 1 exit /b %errorlevel%',
  'call npm.cmd run sync-version',
  'if errorlevel 1 exit /b %errorlevel%',
  `call "${tauriCliPath}" ${quotedArgs.join(' ')}`.trim(),
].join('\r\n');

fs.writeFileSync(tempScriptPath, scriptBody);

try {
  runFinal('cmd.exe', ['/d', '/c', tempScriptPath]);
} finally {
  fs.rmSync(tempScriptPath, { force: true });
}
