import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./static/setup-apps.js', import.meta.url), 'utf8');
const { getSetupPlatforms, getSetupApp } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);

test('starred built-in and custom clients lead each supported platform in stable order', () => {
  const settings = {
    includeBuiltIns: true,
    priorityBuiltIns: ['incy'],
    clients: [
      { id: 'first', name: 'First', scheme: 'first://add/', enabled: true, featured: true, allPlatforms: true },
      { id: 'second', name: 'Second', scheme: 'second://add/', enabled: true, featured: true, allPlatforms: false, platforms: ['windows'] },
      { id: 'regular', name: 'Regular', scheme: 'regular://add/', enabled: true, featured: false, allPlatforms: true },
    ],
  };
  const platforms = getSetupPlatforms(settings);
  assert.deepEqual(platforms.find((platform) => platform.id === 'windows').apps.map((app) => app.id), ['incy', 'first', 'second', 'happ', 'regular']);
  assert.deepEqual(platforms.find((platform) => platform.id === 'ios').apps.map((app) => app.id), ['incy', 'first', 'happ', 'regular']);
  assert.equal(getSetupApp('windows', '', settings).id, 'incy');
});

test('a starred custom client appears before unstarred built-ins', () => {
  const settings = {
    includeBuiltIns: true,
    priorityBuiltIns: [],
    clients: [{ id: 'first', name: 'First', scheme: 'first://add/', enabled: true, featured: true, allPlatforms: true }],
  };
  assert.deepEqual(getSetupPlatforms(settings)[0].apps.map((app) => app.id), ['first', 'happ', 'incy']);
  assert.equal(getSetupApp('ios', '', settings).id, 'first');
});
