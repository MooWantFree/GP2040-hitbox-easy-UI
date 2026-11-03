import { test } from 'node:test';
import assert from 'node:assert/strict';
import { model } from './load-model.mjs';
const { OUTPUTS, LABEL_SETS, mappingLabel, makeMapping, validateMapping, validateLayout, shapeMembers } = model;

test('platform names preserve the same actions and masks', () => {
  const b1 = makeMapping([5]);
  const labels = ['B1', 'A', '×', 'B', 'K1'];
  Object.keys(LABEL_SETS).forEach((scheme, i) => assert.equal(mappingLabel(b1, scheme), labels[i]));
  const combo = makeMapping([5, 11, 16, 1]);
  assert.deepEqual(combo, { action: 40, customButtonMask: 8257, customDpadMask: 1 });
  assert.equal(mappingLabel(combo, 'playstation'), '↑ + × + L2 + Touchpad');
  assert.equal(mappingLabel(combo, 'switch'), '↑ + B + ZL + Capture');
  for (const scheme of Object.keys(LABEL_SETS)) {
    mappingLabel(combo, scheme);
    for (const output of OUTPUTS) validateMapping(makeMapping([output.action]));
  }
  assert.equal(combo.customButtonMask, 8257);
  assert.throws(() => validateMapping({ action: 40, customButtonMask: 16384, customDpadMask: 0 }));
});

const fixture = () => ({ version: 2, name: '布局', deviceKey: null, labelSet: 'switch', buttons: ['up','down','left','right'].map((direction, i) => ({ id: direction, name: direction, x: 25, y: 50, size: 26, shape: 'joystick', groupId: 'stick', direction, pin: i })) });
test('joystick export and import preserve four distinct bindings and grouping', () => {
  const layout = fixture();
  assert.deepEqual(validateLayout(JSON.parse(JSON.stringify(layout))), layout);
  assert.equal(shapeMembers(layout.buttons, layout.buttons[0]).length, 4);
  const normal = { id: 'normal', name: 'normal', x: 70, y: 50, size: 7, pin: null };
  const legacy = validateLayout({ version: 1, name: '旧布局', deviceKey: null, buttons: [normal] });
  assert.equal(legacy.labelSet, 'xbox'); assert.equal(legacy.buttons[0].shape, 'circle');
  assert.equal(shapeMembers([...layout.buttons, normal], normal).length, 1);
});
test('incomplete, inconsistent, or conflicting joystick imports are rejected', () => {
  for (const mutate of [
    (l) => l.buttons.pop(),
    (l) => l.buttons[1].direction = 'up',
    (l) => l.buttons[1].x = 30,
    (l) => l.buttons[1].size = 30,
    (l) => l.buttons[1].pin = 0,
    (l) => l.buttons[1].shape = 'circle',
    (l) => l.labelSet = 'unknown',
  ]) { const l = fixture(); mutate(l); assert.throws(() => validateLayout(l)); }
});
