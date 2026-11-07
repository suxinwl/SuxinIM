import path from 'node:path';

export function clientManifestOutput(suite, args) {
  let candidate = false, explicit;
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--candidate' && !candidate) candidate = true;
    else if (args[i] === '--candidate-output' && !explicit && args[i + 1]) explicit = args[++i];
    else throw new Error('Unsupported manifest option');
  }
  if (explicit && !candidate) throw new Error('Explicit output requires candidate mode');
  const privateRoot = path.resolve(suite, '.private/production/acceptance');
  const output = path.resolve(suite, explicit || (candidate ? '.private/production/acceptance/client-organization-candidate-artifacts.json' : 'clients/artifacts-manifest.json'));
  if (explicit) {
    const relative = path.relative(privateRoot, output);
    if (!relative || relative.startsWith('..') || path.isAbsolute(relative) || path.extname(output) !== '.json') throw new Error('Candidate output must be private acceptance JSON');
  }
  return { candidate, output };
}
