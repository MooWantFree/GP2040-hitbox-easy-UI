import { t, I18nError } from './i18n.js';
// Values follow GP2040-CE/proto/enums.proto and headers/gamepad/GamepadState.h.
export const OUTPUTS = [
  { action: 1, label: '↑', get name() { return t('direction.up'); }, code: 'Up', dpad: 1 },
  { action: 2, label: '↓', get name() { return t('direction.down'); }, code: 'Down', dpad: 2 },
  { action: 3, label: '←', get name() { return t('direction.left'); }, code: 'Left', dpad: 4 },
  { action: 4, label: '→', get name() { return t('direction.right'); }, code: 'Right', dpad: 8 },
  { action: 5, label: 'A', code: 'B1', mask: 1, color: 'green' },
  { action: 6, label: 'B', code: 'B2', mask: 2, color: 'red' },
  { action: 7, label: 'X', code: 'B3', mask: 4, color: 'blue' },
  { action: 8, label: 'Y', code: 'B4', mask: 8, color: 'yellow' },
  { action: 9, label: 'LB', code: 'L1', mask: 16 },
  { action: 10, label: 'RB', code: 'R1', mask: 32 },
  { action: 11, label: 'LT', code: 'L2', mask: 64 },
  { action: 12, label: 'RT', code: 'R2', mask: 128 },
  { action: 13, label: 'View', name: 'View / Back', code: 'S1', mask: 256 },
  { action: 14, label: 'Menu', name: 'Menu / Start', code: 'S2', mask: 512 },
  { action: 17, label: 'LS', get name() { return t('output.leftStick'); }, code: 'L3', mask: 1024 },
  { action: 18, label: 'RS', get name() { return t('output.rightStick'); }, code: 'R3', mask: 2048 },
  { action: 15, label: 'Guide', name: 'Xbox / Guide', code: 'A1', mask: 4096 },
  { action: 16, label: 'A2', get name() { return t('output.a2Xbox'); }, code: 'A2', mask: 8192 },
  { action: 19, label: 'Fn', code: 'Fn' },
  { action: -10, get label() { return t('output.disabled'); }, code: 'NONE' },
];

// Display conventions only. Action numbers and masks never depend on this choice.
export const LABEL_SETS = { gp2040: 'GP2040', xbox: 'Xbox', playstation: 'PlayStation', switch: 'Switch', arcade: 'Arcade' };
const LABELS = {
  playstation: { B1: '×', B2: '○', B3: '□', B4: '△', S1: 'Share', S2: 'Options', A1: 'PS', A2: 'Touchpad' },
  switch: { B1: 'B', B2: 'A', B3: 'Y', B4: 'X', L1: 'L', R1: 'R', L2: 'ZL', R2: 'ZR', S1: '−', S2: '+', L3: 'LS', R3: 'RS', A1: 'Home', A2: 'Capture' },
  arcade: { B1: 'K1', B2: 'K2', B3: 'P1', B4: 'P2', L1: 'P4', R1: 'P3', L2: 'K4', R2: 'K3', S1: 'Coin', S2: 'Start', L3: 'LS', R3: 'RS', A1: 'Home', A2: 'A2' },
};
export function outputLabel(output, labelSet = 'xbox') {
  if (output.dpad || output.action === -10 || labelSet === 'xbox') return output.label;
  return LABELS[labelSet]?.[output.code] || output.code;
}
export function outputName(output, labelSet = 'xbox') {
  if (output.dpad) return output.name;
  if (labelSet === 'xbox') return output.name || output.label;
  return ({ playstation: { B1: '× / Cross', B2: '○ / Circle', B3: '□ / Square', B4: '△ / Triangle' } })[labelSet]?.[output.code] || outputLabel(output, labelSet);
}
export function outputColor(output, labelSet = 'xbox') {
  if (labelSet === 'xbox') return output.color || '';
  return labelSet === 'playstation' ? ({ B1: 'blue', B2: 'red', B3: 'pink', B4: 'green' })[output.code] || '' : '';
}
export const DIRECTIONS = { get up() { return t('direction.up'); }, get down() { return t('direction.down'); }, get left() { return t('direction.left'); }, get right() { return t('direction.right'); } };
export const DIRECTION_ARROWS = { up: '↑', down: '↓', left: '←', right: '→' };
export const shapeMembers = (buttons, button) => button.groupId ? buttons.filter((b) => b.groupId === button.groupId) : [button];

export const pinKey = (pin) => `pin${String(pin).padStart(2, '0')}`;
export const pinKeys = (profile) => Object.keys(profile).filter((k) => /^pin\d{2}$/.test(k)).sort();
export const protectedPin = (mapping) => !mapping || [0, -5].includes(mapping.action);
export const same = (a, b) => JSON.stringify(canonical(a)) === JSON.stringify(canonical(b));
function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map((k) => [k, canonical(value[k])]));
  return value;
}
export function selectedOutputs(mapping) {
  if (!mapping) return [];
  return OUTPUTS.filter((o) => mapping.action === 40
    ? (o.mask && (mapping.customButtonMask & o.mask)) || (o.dpad && (mapping.customDpadMask & o.dpad))
    : o.action === mapping.action);
}
export const isExtendedMapping = (mapping) => mapping?.action === 40 && Boolean((mapping.customButtonMask & ~16383) || (mapping.customDpadMask & ~15));
export function mappingLabel(mapping, labelSet = 'xbox') {
  if (!mapping) return t('identify.unbound');
  if (mapping.action === 0) return t('output.peripheral');
  if (mapping.action === -5) return t('output.reserved');
  const outputs = selectedOutputs(mapping);
  if (mapping.action === 40 && ((mapping.customButtonMask & ~16383) || (mapping.customDpadMask & ~15))) return t('output.extendedCombo');
  return outputs.map((o) => outputLabel(o, labelSet)).join(' + ') || t('output.advanced' , mapping.action);
}
export function makeMapping(actions) {
  const options = [...new Set(actions)].map((a) => OUTPUTS.find((o) => o.action === a));
  if (!options.length || options.some((o) => !o)) throw new I18nError('output.choose');
  if (options.length === 1) return { action: options[0].action, customButtonMask: 0, customDpadMask: 0 };
  if (options.some((o) => !o.mask && !o.dpad)) throw new I18nError('output.comboInvalid');
  return {
    action: 40,
    customButtonMask: options.reduce((mask, o) => mask | (o.mask || 0), 0),
    customDpadMask: options.reduce((mask, o) => mask | (o.dpad || 0), 0),
  };
}
export function validateMapping(mapping) {
  if (!mapping || !Number.isInteger(mapping.action) || !Number.isInteger(mapping.customButtonMask) || !Number.isInteger(mapping.customDpadMask)) throw new I18nError('error.buttonMapping');
  if (mapping.action === 40) {
    if (mapping.customButtonMask < 0 || mapping.customButtonMask > 16383 || mapping.customDpadMask < 0 || mapping.customDpadMask > 15 || !(mapping.customButtonMask || mapping.customDpadMask)) throw new I18nError('error.combo');
  } else if (!OUTPUTS.some((o) => o.action === mapping.action) || mapping.customButtonMask !== 0 || mapping.customDpadMask !== 0) throw new I18nError('output.unsupported');
}
export function profileChanges(original, draft) {
  return Object.fromEntries(pinKeys(original).filter((key) => !same(original[key], draft[key])).map((key) => [key, draft[key]]));
}
export function validateLayout(value) {
  if (!value || ![1, 2].includes(value.version) || typeof value.name !== 'string' || value.name.length > 80 || !Array.isArray(value.buttons) || value.buttons.length > 80) throw new I18nError('error.layoutImport');
  if (value.deviceKey !== null && (typeof value.deviceKey !== 'string' || value.deviceKey.length > 1000)) throw new I18nError('error.binding');
  const labelSet = value.labelSet ?? 'xbox';
  if (!Object.hasOwn(LABEL_SETS, labelSet)) throw new I18nError('error.labelSet');
  const ids = new Set(), pins = new Set(), groups = new Map();
  for (const b of value.buttons) {
    if (!b || typeof b.id !== 'string' || !/^[\w-]{1,80}$/.test(b.id) || ids.has(b.id) || typeof b.name !== 'string' || b.name.length > 40 || !Number.isFinite(b.x) || !Number.isFinite(b.y) || b.x < 0 || b.x > 100 || b.y < 0 || b.y > 100 || !Number.isFinite(b.size) || b.size < (b.shape === 'joystick' ? 12 : 2) || b.size > (b.shape === 'joystick' ? 40 : 20)) throw new I18nError('error.layoutPosition');
    if (b.pin !== null && (!Number.isInteger(b.pin) || b.pin < 0 || b.pin > 47 || pins.has(b.pin))) throw new I18nError('error.duplicatePhysical');
    if (b.shape !== undefined && !['dot', 'circle', 'capsule', 'joystick'].includes(b.shape)) throw new I18nError('error.shape');
    if (b.shape === 'joystick') {
      if (typeof b.groupId !== 'string' || !/^[\w-]{1,80}$/.test(b.groupId) || !Object.hasOwn(DIRECTIONS, b.direction)) throw new I18nError('error.stickDirection');
      const group = groups.get(b.groupId) || [];
      if (group.some((other) => other.direction === b.direction || other.x !== b.x || other.y !== b.y || other.size !== b.size)) throw new I18nError('error.stickConsistency');
      group.push(b); groups.set(b.groupId, group);
    } else if (b.groupId !== undefined || b.direction !== undefined) throw new I18nError('error.normalDirection');
    ids.add(b.id);
    if (b.pin !== null) pins.add(b.pin);
  }
  if ([...groups.values()].some((group) => group.length !== 4)) throw new I18nError('error.stickIncomplete');
  return { version: 2, name: value.name, deviceKey: value.deviceKey, labelSet, buttons: value.buttons.map(({id,name,x,y,size,pin,shape = 'circle',groupId,direction}) => ({id,name,x,y,size,pin,shape,...(shape === 'joystick' ? {groupId,direction} : {})})) };
}
