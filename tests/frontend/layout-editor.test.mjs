import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
const source = await readFile(new URL('../../web/layout-editor.js', import.meta.url), 'utf8');
const { shapeKey, selectionShapes, selectionMembers, movementDelta, scaleRange } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
const circle = (id, x, y, size = 10) => ({ id, x, y, size, shape: 'circle', pin: null });

test('selection keeps joystick directions together without merging a same-named button', () => {
  const stick = ['up','down','left','right'].map((direction, i) => ({ ...circle(direction, 25, 50, 26), shape: 'joystick', groupId: 'stick', direction, pin: i }));
  const normal = circle('stick', 70, 50);
  const buttons = [...stick, normal, circle('other', 85, 50)];
  const keys = new Set([shapeKey(stick[1]), shapeKey(normal)]);
  assert.equal(selectionShapes(buttons, keys).length, 2);
  assert.deepEqual(selectionMembers(buttons, keys).map((b) => b.id), ['up','down','left','right','stick']);
  const remaining = buttons.filter((b) => !keys.has(shapeKey(b)));
  assert.deepEqual(remaining.map((b) => b.id), ['other']);
  assert.deepEqual(stick.map((b) => b.pin), [0,1,2,3]);
});

test('group movement stops at the first edge without changing relative positions', () => {
  const shapes = [circle('left', 20, 35), circle('right', 90, 65)];
  assert.deepEqual(movementDelta(shapes, 30, 30, 2), { x: 5, y: 25 });
  assert.deepEqual(movementDelta(shapes, -30, -30, 2), { x: -15, y: -25 });
  const delta = movementDelta(shapes, 30, 30, 2);
  const moved = shapes.map((b) => ({ ...b, x: b.x + delta.x, y: b.y + delta.y }));
  assert.equal(moved[1].x - moved[0].x, 70);
  assert.equal(moved[1].y - moved[0].y, 30);
  assert.deepEqual(movementDelta(moved, .3, .3, 2), { x: 0, y: 0 });
});

test('mixed shapes respect their height and preserve keyboard nudge distances', () => {
  const shapes = [{ ...circle('capsule', 25, 80, 12), shape: 'capsule' }, { ...circle('stick', 60, 45, 26), shape: 'joystick' }];
  assert.deepEqual(movementDelta(shapes, .3, -.3, 2), { x: .3, y: -.3 });
  assert.deepEqual(movementDelta(shapes, 2, -2, 2), { x: 2, y: -2 });
  assert.deepEqual(movementDelta(shapes, 0, 30, 2), { x: 0, y: 15 });
  assert.deepEqual(movementDelta([], 20, 20, 2), { x: 0, y: 0 });
});

test('batch scale range keeps mixed shapes within valid size limits', () => {
  const shapes = [circle('dot', 20, 25, 4), circle('circle', 50, 50, 7), { ...circle('stick', 75, 50, 26), shape: 'joystick' }];
  const range = scaleRange(shapes);
  assert.deepEqual(range, { min: 50, max: 153 });
  for (const percent of [range.min, 100, range.max]) {
    const sizes = shapes.map((b) => b.size * percent / 100);
    shapes.forEach((b, i) => assert.ok(sizes[i] >= (b.shape === 'joystick' ? 12 : 2) && sizes[i] <= (b.shape === 'joystick' ? 40 : 20)));
    assert.ok(Math.abs(sizes[0] / sizes[1] - 4 / 7) < 1e-10);
  }
  assert.deepEqual(scaleRange([circle('small', 20, 50, 2), { ...circle('large', 65, 50, 40), shape: 'joystick' }]), { min: 100, max: 100 });
});
