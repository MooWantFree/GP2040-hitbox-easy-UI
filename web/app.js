import { OUTPUTS, LABEL_SETS, DIRECTIONS, DIRECTION_ARROWS, shapeMembers, outputLabel, outputName, outputColor, pinKey, pinKeys, protectedPin, selectedOutputs, isExtendedMapping, mappingLabel as describeMapping, makeMapping, profileChanges, validateLayout, validateMapping } from './model.js';
import { t, I18nError, errorText, resolveLanguage, currentLanguage, setLanguage, availableLanguages, languageWarnings, loadLanguages, translatePage } from './i18n.js';
import { shapeKey, selectionShapes, selectionMembers, movementDelta, scaleRange } from './layout-editor.js';

const browserLanguages = () => navigator.languages?.length ? navigator.languages : [navigator.language || 'en'];
let languagePreference = 'auto';
try { languagePreference = localStorage.getItem('hitbox.language') || 'auto'; } catch {}
try { await loadLanguages(languagePreference === 'auto' ? browserLanguages() : [languagePreference]); }
catch (error) { const el = document.getElementById('message'); el.textContent = error.message; el.hidden = false; throw error; }
translatePage();

const $ = (id) => document.getElementById(id);
const SHAPES = { dot: { size: 4, ratio: 1 }, circle: { size: 7, ratio: 1 }, capsule: { size: 11, ratio: 2.4 }, joystick: { size: 26, ratio: 1 } };
const positions = [[20,48,7],[28,43,7],[36,48,7],[43,78,9],[56,38,7],[65,33,7],[74,35,7],[83,42,7],[56,59,7],[65,54,7],[74,56,7],[83,63,7],[23,20,4],[30,18,4],[37,19,4],[44,21,4]];
let layout = { version: 2, name: t('layout.name'), deviceKey: null, labelSet: 'xbox', buttons: positions.map(([x,y,size], i) => ({ id: crypto.randomUUID(), name: t('layout.position' , i + 1), x,y,size,shape: size < 5 ? 'dot' : 'circle',pin: null })) };
let connection = null;
let drafts = [];
let profileIndex = 0;
let selectedId = null;
let editSelection = new Set();
let resizeBasis = null;
let mode = 'map';
let busy = false;
let swapFrom = null;
let captureCancelled = false;
let lastBackup = null;
let layoutQueue = Promise.resolve();
let layoutSaving = 0;
let layoutLoadFailed = false;
let dragging = null;
let suppressClick = false;
let suppressClickTimer;
let layoutSaved = false;
let lastMessage = '', lastMessageType = '', layoutError = null;

const selected = () => layout.buttons.find((b) => b.id === selectedId);
const editingShapes = () => selectionShapes(layout.buttons, editSelection);
const isSelected = (button) => mode === 'edit' ? editSelection.has(shapeKey(button)) : button.id === selectedId;
const mappingLabel = (mapping) => describeMapping(mapping, layout.labelSet);
const members = (button) => shapeMembers(layout.buttons, button);
const updateShape = (button, values) => members(button).forEach((b) => Object.assign(b, values));
const hotspotFor = (id) => $('hotspots').querySelector(`[data-button-id="${id}"]`);
const shapeElement = (button) => button.groupId ? $('hotspots').querySelector(`[data-group-id="${button.groupId}"]`) : hotspotFor(button.id);
const matchingDevice = () => connection && (!layout.deviceKey || layout.deviceKey === connection.deviceKey);
const boundDevice = () => connection && layout.deviceKey === connection.deviceKey;
const mappingFor = (button, index = profileIndex) => boundDevice() && button?.pin !== null ? drafts[index]?.[pinKey(button?.pin)] : undefined;
const displayOutput = (button) => mappingFor(button) ? mappingLabel(mappingFor(button)) : button?.pin !== null ? t('identify.pending') : t('identify.unbound');
const changes = () => connection ? profileChanges(connection.snapshot.profiles[profileIndex], drafts[profileIndex]) : {};
const allDirty = () => connection && drafts.some((p, i) => Object.keys(profileChanges(connection.snapshot.profiles[i], p)).length);
const isEditable = (button) => matchingDevice() && button?.pin !== null && !protectedPin(mappingFor(button)) && !protectedPin(connection.snapshot.profiles[0][pinKey(button.pin)]);

function message(text, type = '') {
  lastMessage = text; lastMessageType = type;
  text = typeof text === 'function' ? text() : text;
  $('message').textContent = text;
  $('message').className = `message ${type}`;
  $('message').hidden = !text;
}
async function api(path, data, method = 'POST') {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 105000);
  try {
    const response = await fetch(`/api/${path}`, { method, headers: { 'X-Hitbox-Local': '1', ...(method !== 'GET' ? { 'Content-Type': 'application/json' } : {}) }, body: method === 'GET' ? undefined : JSON.stringify(data), signal: controller.signal });
    const result = await response.json();
    if (!response.ok) throw Object.assign(new Error(result.error || t('error.request')), { ...result, status: response.status });
    return result;
  } catch (error) {
    if (error.name === 'AbortError') throw new I18nError('error.timeout');
    throw error;
  } finally { clearTimeout(timer); }
}
function run(action) {
  return async (event) => { try { await action(event); } catch (error) { message(() => errorText(error), 'error'); } };
}
function askConfirm(title, text, accept = t('common.confirm')) {
  const dialog = $('confirmDialog');
  $('confirmTitle').textContent = title; $('confirmText').textContent = text; $('confirmAccept').textContent = accept;
  return new Promise((resolve) => {
    dialog.returnValue = '';
    dialog.addEventListener('close', () => resolve(dialog.returnValue === 'accept'), { once: true });
    dialog.showModal(); $('confirmCancel').focus();
  });
}
$('confirmCancel').addEventListener('click', () => $('confirmDialog').close('cancel'));
$('confirmAccept').addEventListener('click', () => $('confirmDialog').close('accept'));
function persistLayout() {
  if (layoutLoadFailed) return Promise.reject(new I18nError('layout.readBlocked'));
  const copy = structuredClone(layout);
  layoutSaving++;
  $('localStatus').hidden = false;
  $('localStatus').textContent = t('layout.saving');
  layoutQueue = layoutQueue.catch(() => {}).then(() => api('layout', copy, 'PUT'));
  return layoutQueue.then(() => {
    layoutSaved = true; layoutError = null;
    $('layoutSaveError').hidden = true;
  }).catch((error) => {
    layoutError = error;
    $('layoutSaveError').hidden = false;
    $('layoutSaveError').textContent = t('layout.saveError', errorText(error));
    throw error;
  }).finally(() => {
    layoutSaving--;
    $('localStatus').textContent = layoutSaving ? t('layout.saving') : $('layoutSaveError').hidden ? t('layout.saved') : t('layout.unsaved');
  });
}
function download(value, name) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' }));
  const a = document.createElement('a');
  a.href = url; a.download = name; a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function setBusy(value) { busy = value; render(); }
function profileName(index) { return connection?.snapshot.profiles[index]?.profileLabel || t('profile.name' , index + 1); }

function render() {
  const button = selected();
  const selection = editingShapes();
  const multiple = mode === 'edit' && selection.length > 1;
  const draftChanges = changes();
  const dirty = Object.keys(draftChanges).length;
  const linked = Boolean(connection);
  $('language').disabled = busy;
  $('localStatus').textContent = layoutSaving ? t('layout.saving') : layoutError ? t('layout.unsaved') : layoutSaved ? t('layout.saved') : t('layout.autosave');
  if (layoutError) $('layoutSaveError').textContent = t('layout.saveError', errorText(layoutError));
  $('connectionText').textContent = linked ? t('connection.connected') : t('connection.disconnected');
  $('connectionDot').classList.toggle('connected', linked);
  $('connect').disabled = busy;
  $('connect').textContent = busy ? t('connection.busy') : linked ? t('connection.reload') : t('connection.connect');
  $('target').disabled = busy;
  $('backup').disabled = !linked || busy;
  $('deviceMismatch').hidden = !connection || matchingDevice();
  $('rebindDevice').disabled = busy;
  $('profile').disabled = !linked || busy;
  const canCopy = linked && drafts.length > 1 && matchingDevice() && !busy;
  $('copySource').disabled = !canCopy;
  $('copyProfile').disabled = !canCopy;
  if (linked) {
    for (const option of $('copySource').options) option.disabled = Number(option.value) === profileIndex;
    if (Number($('copySource').value) === profileIndex) $('copySource').value = drafts.findIndex((_, index) => index !== profileIndex);
  }
  $('profileNote').hidden = !linked;
  $('profileNote').textContent = linked ? t('profile.active' , profileName(Math.max(0, (connection.snapshot.options.profileNumber || 1) - 1))) : '';
  $('dirtyCount').textContent = dirty ? t('changes.count' , dirty) : allDirty() ? t('changes.other') : t('changes.none');
  $('dirtyCount').classList.toggle('pending', Boolean(allDirty()));
  $('review').disabled = !dirty || busy || !matchingDevice();
  $('discard').disabled = !dirty || busy;
  $('downloadBackup').hidden = !lastBackup;
  $('boundCount').textContent = t('layout.bound' , layout.buttons.filter((b) => b.pin !== null).length, layout.buttons.length);
  $('editTools').hidden = mode !== 'edit';
  $('selectionCount').textContent = t('layout.selected', selection.length);
  $('selectionCount').hidden = !selection.length;
  $('sizeControl').hidden = !selection.length;
  $('shapePalette').hidden = mode !== 'edit';
  $('canvasEditor').classList.toggle('with-palette', mode === 'edit');
  $('deleteButton').disabled = !selection.length || busy;
  $('deleteButton').hidden = !selection.length;
  $('deleteButton').textContent = selection.length === 1 && button?.groupId ? t('joystick.delete') : t('common.delete');
  $('buttonSize').disabled = !selection.length || busy;
  $('sizeValue').hidden = !multiple;
  if (multiple) {
    const key = JSON.stringify(selection.map(shapeKey).sort());
    if (resizeBasis?.key !== key) resizeBasis = { key, shapes: selection.map((b) => ({ ...b })), value: 100 };
    const range = scaleRange(resizeBasis.shapes);
    Object.assign($('buttonSize'), { min: range.min, max: range.max, step: 1, value: resizeBasis.value });
    $('buttonSize').disabled ||= range.min === range.max;
    $('buttonSize').setAttribute('aria-valuetext', `${resizeBasis.value}%`);
    $('sizeValue').textContent = `${resizeBasis.value}%`;
  } else {
    resizeBasis = null;
    $('buttonSize').removeAttribute('aria-valuetext');
    $('buttonSize').step = .2;
  }
  $('board').classList.toggle('editing', mode === 'edit');
  for (const el of document.querySelectorAll('[data-mode]')) {
    el.classList.toggle('active', mode === el.dataset.mode);
    el.setAttribute('aria-pressed', String(mode === el.dataset.mode));
    el.disabled = busy;
  }
  for (const id of ['importLayout']) $(id).disabled = busy;
  for (const el of document.querySelectorAll('[data-shape]')) el.disabled = busy || layout.buttons.length + (el.dataset.shape === 'joystick' ? 4 : 1) > 80;
  for (const el of document.querySelectorAll('[data-label-set]')) {
    const active = el.dataset.labelSet === layout.labelSet;
    el.classList.toggle('active', active); el.setAttribute('aria-pressed', String(active)); el.disabled = busy;
  }
  $('outputLabelSet').textContent = LABEL_SETS[layout.labelSet];
  for (const el of $('outputs').children) {
    const output = OUTPUTS.find((o) => o.action === Number(el.dataset.action));
    el.textContent = outputLabel(output, layout.labelSet);
    el.title = outputName(output, layout.labelSet);
    el.setAttribute('aria-label', t('output.accessible' , outputName(output, layout.labelSet)));
    el.className = outputColor(output, layout.labelSet);
  }
  $('modeHint').textContent = swapFrom ? t('swap.hint')
    : mode === 'edit' ? t('layout.moveHint')
    : mode === 'identify' ? button?.groupId ? t('identify.stickHint') : t('identify.hint') : '';
  $('modeHint').hidden = !$('modeHint').textContent;
  renderBoard(draftChanges);
  $('noSelection').hidden = Boolean(button);
  $('selectionPanel').hidden = !button || multiple;
  if (!button || multiple) return;
  if (document.activeElement !== $('buttonName')) $('buttonName').value = button.name;
  $('buttonName').disabled = busy;
  $('buttonSize').min = button.shape === 'joystick' ? 12 : 2;
  $('buttonSize').max = button.shape === 'joystick' ? 40 : 20;
  $('buttonSize').value = button.size;
  $('selectedNumber').textContent = String(layout.buttons.indexOf(button) + 1).padStart(2, '0');
  const mapping = mappingFor(button);
  $('currentOutput').textContent = displayOutput(button);
  $('identifySelected').textContent = button.pin === null ? t('identify.start') : t('identify.again');
  $('identifySelected').disabled = !linked || busy || !matchingDevice();
  $('selectionHint').textContent = !linked ? t('connection.required')
    : !matchingDevice() ? t('device.rebindFirst')
    : button.pin === null ? ''
    : protectedPin(mapping) ? t('selection.protected')
    : selectedOutputs(mapping).length === 0 || isExtendedMapping(mapping) ? t('output.advancedHint') : '';
  $('selectionHint').hidden = !$('selectionHint').textContent;
  $('outputPanel').hidden = mode !== 'map' || !boundDevice() || button.pin === null;
  $('pinDetail').textContent = button.pin === null ? t('identify.notYet') : `GPIO ${button.pin} · ${selectedOutputs(mapping).map((o) => o.code).join(' + ') || t('output.advancedCurrent')}`;
  const enabled = isEditable(button) && !busy && mode === 'map';
  $('combo').disabled = !enabled;
  $('comboHint').hidden = !$('combo').checked;
  $('swap').disabled = !enabled;
  $('swap').textContent = swapFrom ? t('swap.cancel') : t('swap.start');
  $('resetKey').disabled = !enabled || !draftChanges[pinKey(button.pin)];
  for (const el of $('outputs').children) {
    const action = Number(el.dataset.action);
    el.disabled = !enabled;
    const active = selectedOutputs(mapping).some((o) => o.action === action);
    el.classList.toggle('active', active);
    el.setAttribute('aria-pressed', String(active));
  }
}

function renderBoard(draftChanges) {
  const focusedId = document.activeElement?.dataset?.buttonId;
  const focusedGroup = document.activeElement?.dataset?.gripId;
  const focusedChip = document.activeElement?.dataset?.chipId;
  const hotspots = document.createDocumentFragment();
  const list = document.createDocumentFragment();
  const groups = new Map();
  layout.buttons.forEach((b, i) => {
    const mapping = mappingFor(b);
    const outputs = selectedOutputs(mapping);
    const el = document.createElement('button');
    el.type = 'button'; el.dataset.buttonId = b.id;
    el.className = `hotspot shape-${b.shape} ${b.direction ? `direction-${b.direction}` : ''} ${isSelected(b) ? 'selected' : ''} ${b.pin === null ? 'unbound' : ''} ${draftChanges[pinKey(b.pin)] ? 'changed' : ''} ${outputs.length === 1 ? outputColor(outputs[0], layout.labelSet) : ''} ${swapFrom && b.id !== swapFrom ? 'swap-target' : ''}`;
    if (!b.groupId) Object.assign(el.style, { left: `${b.x}%`, top: `${b.y}%`, width: `${b.size}%`, aspectRatio: SHAPES[b.shape].ratio });
    el.setAttribute('aria-label', `${b.name}, ${displayOutput(b)}`);
    el.title = `${b.name} · ${displayOutput(b)}`;
    el.setAttribute('aria-pressed', String(isSelected(b)));
    el.disabled = busy;
    const number = document.createElement('span'); number.className = 'key-index'; number.textContent = b.direction ? DIRECTION_ARROWS[b.direction] : String(i + 1).padStart(2, '0');
    number.hidden = Boolean(b.direction && (!mapping || mappingLabel(mapping) === DIRECTION_ARROWS[b.direction]));
    const label = document.createElement('span'); label.className = 'key-output'; label.textContent = mapping ? mappingLabel(mapping) : b.direction ? DIRECTION_ARROWS[b.direction] : b.shape === 'dot' ? String(i + 1).padStart(2, '0') : '·';
    el.append(number, label);
    el.addEventListener('click', run(() => { if (!suppressClick) return choose(b.id); }));
    el.addEventListener('pointerdown', (event) => startDrag(event, b));
    el.addEventListener('lostpointercapture', cancelPointerDrag);
    el.addEventListener('keydown', run((event) => editKeydown(event, b)));
    if (b.groupId) {
      if (!groups.has(b.groupId)) {
        const group = document.createElement('div'); group.className = 'joystick'; group.dataset.groupId = b.groupId;
        Object.assign(group.style, { left: `${b.x}%`, top: `${b.y}%`, width: `${b.size}%` });
        const grip = document.createElement('button'); grip.type = 'button'; grip.className = 'joystick-grip'; grip.dataset.gripId = b.groupId;
        grip.setAttribute('aria-label', t('joystick.move')); grip.title = t('joystick.moveHint'); grip.disabled = busy || mode !== 'edit';
        grip.addEventListener('pointerdown', (event) => startDrag(event, b));
        grip.setAttribute('aria-pressed', String(isSelected(b)));
        grip.addEventListener('click', run(() => { if (!suppressClick) return choose(b.id); }));
        grip.addEventListener('keydown', run((event) => editKeydown(event, b)));
        group.addEventListener('lostpointercapture', cancelPointerDrag);
        group.append(grip); groups.set(b.groupId, group); hotspots.append(group);
      }
      groups.get(b.groupId).append(el);
    } else hotspots.append(el);
    const chip = document.createElement('button');
    chip.className = isSelected(b) ? 'active' : ''; chip.disabled = busy;
    chip.dataset.chipId = b.id; chip.setAttribute('aria-pressed', String(isSelected(b)));
    chip.addEventListener('keydown', run((event) => editKeydown(event, b)));
    const index = document.createElement('span'); index.className = 'mini-index'; index.textContent = String(i + 1).padStart(2, '0');
    const name = document.createElement('span'); name.textContent = b.name;
    const output = document.createElement('span'); output.className = 'mini-output'; output.textContent = displayOutput(b);
    chip.append(index,name,output); chip.addEventListener('click', run(() => choose(b.id))); list.append(chip);
  });
  $('hotspots').replaceChildren(hotspots); $('keyList').replaceChildren(list);
  if (focusedId) hotspotFor(focusedId)?.focus({ preventScroll: true });
  else if (focusedGroup) $('hotspots').querySelector(`[data-grip-id="${focusedGroup}"]`)?.focus({ preventScroll: true });
  else if (focusedChip) $('keyList').querySelector(`[data-chip-id="${focusedChip}"]`)?.focus({ preventScroll: true });
}

async function choose(id) {
  if (busy) return;
  const button = layout.buttons.find((b) => b.id === id);
  if (mode === 'edit') {
    const key = shapeKey(button);
    if (editSelection.has(key)) editSelection.delete(key);
    else editSelection.add(key);
    selectedId = editSelection.has(key) ? id : editingShapes().at(-1)?.id ?? null;
    render();
    return;
  }
  if (swapFrom) {
    const source = layout.buttons.find((b) => b.id === swapFrom);
    if (source.id === id) { swapFrom = null; render(); return; }
    if (!isEditable(source) || !isEditable(button)) throw new I18nError('swap.invalid');
    // Keep advanced mappings intact; the supported writer must validate both sides.
    const sourceMapping = mappingFor(source), targetMapping = mappingFor(button);
    if (![sourceMapping, targetMapping].every((m) => selectedOutputs(m).length && !isExtendedMapping(m))) throw new I18nError('swap.advanced');
    [drafts[profileIndex][pinKey(source.pin)], drafts[profileIndex][pinKey(button.pin)]] = [structuredClone(targetMapping), structuredClone(sourceMapping)];
    swapFrom = null; selectedId = id; render(); return;
  }
  selectedId = id;
  $('combo').checked = mappingFor(button)?.action === 40;
  render();
  if (mode === 'identify' && matchingDevice()) await capture();
}

async function capture() {
  const button = selected();
  if (!button || !connection || busy || !matchingDevice()) return;
  if (allDirty()) throw new I18nError('identify.dirty');
  const token = connection.token;
  captureCancelled = false;
  $('captureTitle').textContent = t('identify.namedTitle' , button.name);
  if (button.direction) $('captureTitle').textContent = t('identify.stickTitle' , DIRECTIONS[button.direction]);
  $('captureHint').textContent = button.direction ? t('identify.stickInstructions') : t('identify.instructions');
  $('captureDialog').showModal(); setBusy(true);
  try {
    const { heldPins } = await api('capture', { token });
    if (captureCancelled || connection?.token !== token) return;
    if (!heldPins.length) throw new I18nError('identify.none');
    if (heldPins.length !== 1) throw new I18nError('identify.multiple');
    const pin = heldPins[0];
    if (protectedPin(connection.snapshot.profiles[0][pinKey(pin)])) throw new I18nError('identify.protected');
    const duplicate = layout.buttons.find((b) => b.id !== button.id && b.pin === pin);
    if (duplicate) {
      $('captureDialog').close();
      const overwrite = await askConfirm(t('identify.overwriteTitle'), t('identify.overwriteBody' , duplicate.name, button.name, duplicate.name), t('identify.overwrite'));
      if (!overwrite || connection?.token !== token) return;
      duplicate.pin = null;
    }
    button.pin = pin; layout.deviceKey = connection.deviceKey;
    await persistLayout();
    message(() => t('identify.done' , button.name));
  } finally {
    $('captureDialog').close(); setBusy(false);
  }
}

function canvasBounds() {
  const board = $('board'), rect = board.getBoundingClientRect();
  return { left: rect.left + board.clientLeft, top: rect.top + board.clientTop, width: board.clientWidth, height: board.clientHeight };
}
function clampPosition(button, x, y) {
  const rect = canvasBounds();
  const halfWidth = button.size / 2;
  const halfHeight = halfWidth / SHAPES[button.shape].ratio * rect.width / rect.height;
  return { x: Math.max(halfWidth, Math.min(100 - halfWidth, x)), y: Math.max(halfHeight, Math.min(100 - halfHeight, y)) };
}
function dropPosition(event, button) {
  const rect = canvasBounds();
  const x = (event.clientX - rect.left) / rect.width * 100;
  const y = (event.clientY - rect.top) / rect.height * 100;
  return x >= 0 && x <= 100 && y >= 0 && y <= 100 ? clampPosition(button, x, y) : null;
}
async function editKeydown(event, button) {
  if (mode !== 'edit' || busy || dragging) return;
  if (!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Delete'].includes(event.key)) return;
  event.preventDefault();
  if (!editSelection.has(shapeKey(button))) editSelection = new Set([shapeKey(button)]);
  selectedId = button.id;
  if (event.key === 'Delete') { if (!event.repeat) await deleteSelection(); return; }
  const step = event.shiftKey ? 2 : .3;
  const shapes = editingShapes(), rect = canvasBounds();
  const delta = movementDelta(shapes,
    event.key === 'ArrowRight' ? step : event.key === 'ArrowLeft' ? -step : 0,
    event.key === 'ArrowDown' ? step : event.key === 'ArrowUp' ? -step : 0, rect.width / rect.height);
  for (const shape of shapes) updateShape(shape, { x: shape.x + delta.x, y: shape.y + delta.y });
  render();
  if (delta.x || delta.y) await persistLayout();
}
function clearSelection() {
  editSelection.clear(); selectedId = null; render();
  $('board').focus({ preventScroll: true });
}
async function deleteSelection() {
  const shapes = editingShapes();
  if (mode !== 'edit' || busy || !shapes.length || $('confirmDialog').open) return;
  const button = shapes[0];
  const title = shapes.length > 1 ? t('layout.deleteSelectedTitle', shapes.length)
    : button.groupId ? t('joystick.deleteTitle') : t('layout.deleteTitle', button.name);
  const body = shapes.length > 1 ? t('layout.deleteSelectedBody')
    : button.groupId ? t('joystick.deleteBody') : t('layout.deleteBody');
  const removed = new Set(selectionMembers(layout.buttons, editSelection).map((b) => b.id));
  if (!await askConfirm(title, body, t('common.delete'))) return;
  layout.buttons = layout.buttons.filter((b) => !removed.has(b.id));
  clearSelection(); await persistLayout();
}
async function addShape(shape, x, y) {
  if (mode !== 'edit' || busy) return;
  if (layout.buttons.length + (shape === 'joystick' ? 4 : 1) > 80) throw new I18nError('layout.limit');
  const button = { id: crypto.randomUUID(), name: t('layout.position' , layout.buttons.length + 1), x, y, size: SHAPES[shape].size, shape, pin: null };
  Object.assign(button, clampPosition(button, x, y));
  if (shape === 'joystick') {
    const groupId = crypto.randomUUID();
    const number = new Set(layout.buttons.filter((b) => b.groupId).map((b) => b.groupId)).size + 1;
    for (const [direction, name] of Object.entries(DIRECTIONS)) layout.buttons.push({ ...button, id: direction === 'up' ? button.id : crypto.randomUUID(), name: t('joystick.name' , number, name), groupId, direction });
  } else layout.buttons.push(button);
  selectedId = button.id; editSelection = new Set([shapeKey(layout.buttons.find((b) => b.id === button.id))]); render();
  hotspotFor(button.id)?.focus({ preventScroll: true });
  await persistLayout();
}
function startDrag(event, button, shape) {
  if (mode !== 'edit' || busy || dragging || event.button !== 0 || !event.isPrimary) return;
  if (shape && layout.buttons.length + (shape === 'joystick' ? 4 : 1) > 80) return;
  event.preventDefault();
  // Finish editing the previous key before a drop changes the selection.
  if (document.activeElement === $('buttonName')) $('buttonName').blur();
  const element = shape ? event.currentTarget : event.currentTarget.dataset.gripId
    ? $('hotspots').querySelector(`[data-grip-id="${button.groupId}"]`) : hotspotFor(button.id);
  if (!element) return;
  // preventDefault suppresses mouse focus; restore it before capturing the pointer.
  element.focus({ preventScroll: true });
  const keys = button && editSelection.has(shapeKey(button)) ? editSelection : new Set(button ? [shapeKey(button)] : []);
  const origins = selectionShapes(layout.buttons, keys).map((b) => ({ ...b }));
  dragging = { type: shape ? 'new' : 'move', shape, button: button || { shape, size: SHAPES[shape].size }, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, origins, keys: new Set(keys), selectionBefore: new Set(editSelection), selectedBefore: selectedId, moved: false, element };
  element.setPointerCapture(event.pointerId);
  document.body.classList.add('drag-active');
  if (shape) {
    const preview = document.createElement('div');
    preview.id = 'dragPreview'; preview.className = `hotspot shape-${shape} drag-preview`; preview.setAttribute('aria-hidden', 'true'); preview.hidden = true;
    const mark = document.createElement('span'); mark.className = 'key-output'; mark.textContent = '+'; preview.append(mark);
    preview.style.aspectRatio = SHAPES[shape].ratio;
    document.body.append(preview); dragging.preview = preview;
  }
}
window.addEventListener('pointermove', (event) => {
  if (!dragging || dragging.pointerId !== event.pointerId) return;
  const rect = canvasBounds();
  const dx = event.clientX - dragging.startX, dy = event.clientY - dragging.startY;
  if (!dragging.moved && Math.hypot(dx, dy) > 4) {
    dragging.moved = true;
    if (dragging.type === 'move') {
      editSelection = new Set(dragging.keys); selectedId = dragging.button.id;
      for (const button of layout.buttons) {
        const el = hotspotFor(button.id);
        el?.classList.toggle('selected', isSelected(button));
        el?.setAttribute('aria-pressed', String(isSelected(button)));
      }
    }
  }
  if (!dragging.moved) return;
  const b = dragging.button;
  if (dragging.type === 'new') {
    const position = dropPosition(event, b);
    $('board').classList.toggle('drop-ready', Boolean(position));
    dragging.preview.hidden = false;
    dragging.preview.classList.toggle('outside', !position);
    Object.assign(dragging.preview.style, { width: `${rect.width * b.size / 100}px`, left: `${position ? rect.left + position.x / 100 * rect.width : event.clientX}px`, top: `${position ? rect.top + position.y / 100 * rect.height : event.clientY}px` });
  } else {
    const delta = movementDelta(dragging.origins, dx / rect.width * 100, dy / rect.height * 100, rect.width / rect.height);
    for (const origin of dragging.origins) {
      const button = layout.buttons.find((item) => item.id === origin.id);
      updateShape(button, { x: origin.x + delta.x, y: origin.y + delta.y });
      const el = shapeElement(button);
      el.style.left = `${button.x}%`; el.style.top = `${button.y}%`;
    }
  }
});
function clearDrag() {
  const drag = dragging; dragging = null;
  if (!drag) return null;
  drag.preview?.remove(); $('board').classList.remove('drop-ready'); document.body.classList.remove('drag-active');
  if (drag.element.hasPointerCapture(drag.pointerId)) drag.element.releasePointerCapture(drag.pointerId);
  // Pointer-up handles selection. Ignore the compatibility click that follows it.
  suppressClick = true;
  clearTimeout(suppressClickTimer);
  suppressClickTimer = setTimeout(() => { suppressClick = false; }, 100);
  return drag;
}
function cancelDragging() {
  const drag = clearDrag();
  if (drag?.type === 'move') {
    for (const origin of drag.origins) updateShape(layout.buttons.find((button) => button.id === origin.id), { x: origin.x, y: origin.y });
    editSelection = drag.selectionBefore; selectedId = drag.selectedBefore; render();
  }
}
function cancelPointerDrag(event) { if (dragging?.pointerId === event.pointerId) cancelDragging(); }
window.addEventListener('pointerup', run(async (event) => {
  if (dragging?.pointerId !== event.pointerId) return;
  const drag = clearDrag();
  if (drag.type === 'new') {
    const position = drag.moved && dropPosition(event, drag.button);
    if (position) await addShape(drag.shape, position.x, position.y);
  } else {
    if (drag.moved) { render(); await persistLayout(); }
    else await choose(drag.button.id);
  }
}));
window.addEventListener('pointercancel', cancelPointerDrag);
window.addEventListener('blur', cancelDragging);
window.addEventListener('resize', cancelDragging);
window.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape') return;
  if (dragging) { event.preventDefault(); cancelDragging(); }
  else if (mode === 'edit' && !document.querySelector('dialog[open]') && event.target.closest('#board,#keyList,#editTools')) {
    event.preventDefault(); clearSelection();
  }
});
$('board').addEventListener('click', (event) => {
  if (mode === 'edit' && !busy && !suppressClick && !event.target.closest('button')) clearSelection();
});
for (const el of document.querySelectorAll('[data-shape]')) {
  el.addEventListener('pointerdown', (event) => startDrag(event, null, el.dataset.shape));
  el.addEventListener('lostpointercapture', cancelPointerDrag);
  el.addEventListener('keydown', run(async (event) => {
    if (!['Enter', ' '].includes(event.key)) return;
    event.preventDefault();
    if (!event.repeat) await addShape(el.dataset.shape, 50, 50);
  }));
}

for (const output of OUTPUTS) {
  const button = document.createElement('button');
  button.dataset.action = output.action; button.className = output.color || ''; button.textContent = output.label;
  button.title = output.name || output.label; button.setAttribute('aria-label', t('output.accessible' , output.name || output.label));
  button.addEventListener('click', run(() => {
    const current = selected(); if (!isEditable(current) || busy) return;
    let actions = [output.action];
    if ($('combo').checked && (output.mask !== undefined || output.dpad !== undefined)) {
      const previous = selectedOutputs(mappingFor(current)).filter((o) => o.mask || o.dpad).map((o) => o.action);
      actions = previous.includes(output.action) ? previous.filter((a) => a !== output.action) : [...previous, output.action];
      if (!actions.length) actions = [-10];
    }
    const key = pinKey(current.pin);
    drafts[profileIndex][key] = { ...drafts[profileIndex][key], ...makeMapping(actions) };
    message(''); render();
  }));
  $('outputs').append(button);
}
for (const el of document.querySelectorAll('[data-mode]')) el.addEventListener('click', () => {
  cancelDragging();
  if (mode !== el.dataset.mode) editSelection = new Set(selected() ? [shapeKey(selected())] : []);
  mode = el.dataset.mode; swapFrom = null; render();
});
for (const el of document.querySelectorAll('[data-label-set]')) el.addEventListener('click', run(async () => { if (busy || layout.labelSet === el.dataset.labelSet) return; cancelDragging(); layout.labelSet = el.dataset.labelSet; render(); await persistLayout(); }));
$('identifySelected').addEventListener('click', run(capture));
$('captureDialog').addEventListener('close', () => { captureCancelled = true; });
$('captureDialog').addEventListener('cancel', () => { captureCancelled = true; });
$('combo').addEventListener('change', render);
$('swap').addEventListener('click', () => { swapFrom = swapFrom ? null : selectedId; render(); });
$('buttonName').addEventListener('change', run(async () => { if (!selected()) return; selected().name = $('buttonName').value.trim() || t('layout.unnamed'); render(); await persistLayout(); }));
$('buttonSize').addEventListener('input', () => {
  if (mode !== 'edit' || busy || !selected()) return;
  const value = Number($('buttonSize').value);
  const shapes = editingShapes();
  if (shapes.length > 1 && resizeBasis) {
    resizeBasis.value = value;
    for (const button of shapes) {
      const original = resizeBasis.shapes.find((b) => shapeKey(b) === shapeKey(button));
      updateShape(button, { size: original.size * value / 100 });
      updateShape(button, clampPosition(button, button.x, button.y));
    }
  } else if (shapes.length === 1) {
    updateShape(selected(), { size: value });
    updateShape(selected(), clampPosition(selected(), selected().x, selected().y));
  }
  render();
});
$('buttonSize').addEventListener('change', run(persistLayout));
$('deleteButton').addEventListener('click', run(deleteSelection));
$('rebindDevice').addEventListener('click', run(async () => {
  if (!await askConfirm(t('device.rebindTitle'), t('device.rebindBody'), t('device.clearBindings'))) return;
  layout.buttons.forEach((b) => { b.pin = null; }); layout.deviceKey = connection.deviceKey; mode = 'identify'; render(); await persistLayout();
}));

$('connect').addEventListener('click', run(async () => {
  if (allDirty() && !await askConfirm(t('changes.reloadTitle'), t('changes.reloadBody'), t('connection.reload'))) return;
  setBusy(true); message(() => t('connection.reading')); connection = null; drafts = [];
  try {
    connection = await api('connect', { target: $('target').value });
    drafts = structuredClone(connection.snapshot.profiles); profileIndex = 0;
    const active = Number(connection.snapshot.options.profileNumber || 1) - 1;
    if (drafts[active]) profileIndex = active;
    const options = drafts.map((p, i) => { const option = document.createElement('option'); option.value = i; option.textContent = `${profileName(i)}${p.enabled ? '' : t('profile.disabled')}`; return option; });
    $('profile').replaceChildren(...options); $('profile').value = profileIndex;
    $('copySource').replaceChildren(...options.map((option) => option.cloneNode(true)));
    $('official').href = connection.target;
    $('deviceInfo').textContent = `${connection.snapshot.firmware.boardConfigLabel || connection.snapshot.firmware.boardBuild || 'GP2040-CE'} · ${connection.snapshot.firmware.version}`;
    $('deviceInfo').hidden = false;
    if (matchingDevice() && !layout.buttons.some((b) => b.pin !== null)) mode = 'identify';
    message('');
  } catch (error) { $('deviceInfo').hidden = true; throw new I18nError('connection.webMode', error); }
  finally { swapFrom = null; setBusy(false); }
}));
$('profile').addEventListener('change', () => { profileIndex = Number($('profile').value); swapFrom = null; $('combo').checked = mappingFor(selected())?.action === 40; render(); });
$('copyProfile').addEventListener('click', run(() => {
  const sourceIndex = Number($('copySource').value);
  if (!connection || sourceIndex === profileIndex || !drafts[sourceIndex]) return;
  const target = structuredClone(drafts[profileIndex]);
  for (const key of pinKeys(target)) {
    if (protectedPin(connection.snapshot.profiles[0][key]) || protectedPin(target[key]) || protectedPin(drafts[sourceIndex][key])) continue;
    const source = drafts[sourceIndex][key];
    if (['action', 'customButtonMask', 'customDpadMask'].every((field) => source[field] === target[key][field])) continue;
    try { validateMapping(source); }
    catch { throw new I18nError('profile.unsupportedCopy' , profileName(sourceIndex)); }
    target[key] = { ...target[key], action: source.action, customButtonMask: source.customButtonMask, customDpadMask: source.customDpadMask };
  }
  drafts[profileIndex] = target; swapFrom = null; render(); message(() => t('profile.copied'));
}));
$('resetKey').addEventListener('click', () => { const key = pinKey(selected().pin); drafts[profileIndex][key] = structuredClone(connection.snapshot.profiles[profileIndex][key]); render(); });
$('discard').addEventListener('click', run(async () => { if (await askConfirm(t('changes.discardTitle'), t('changes.discardBody' , profileName(profileIndex)), t('changes.discard'))) { drafts[profileIndex] = structuredClone(connection.snapshot.profiles[profileIndex]); render(); } }));
$('review').addEventListener('click', () => {
  const original = connection.snapshot.profiles[profileIndex];
  const rows = Object.entries(changes()).map(([key, value]) => {
    const row = document.createElement('tr');
    const button = layout.buttons.find((b) => pinKey(b.pin) === key);
    for (const text of [button?.name || `GPIO ${Number(key.slice(3))}`, mappingLabel(original[key]), mappingLabel(value)]) { const cell = document.createElement('td'); cell.textContent = text; row.append(cell); }
    return row;
  });
  $('reviewRows').replaceChildren(...rows); $('reviewTitle').textContent = t('save.profile' , profileName(profileIndex));
  const hasMenu = Object.values(drafts[profileIndex]).some((m) => m?.action === 14 || (m?.action === 40 && (m.customButtonMask & 512)));
  $('reviewWarning').hidden = hasMenu;
  $('reviewWarning').textContent = hasMenu ? '' : t('save.menuWarning' , outputLabel(OUTPUTS.find((o) => o.action === 14), layout.labelSet));
  $('reviewDialog').showModal();
});
$('closeReview').addEventListener('click', () => $('reviewDialog').close());
$('reviewDialog').addEventListener('cancel', (event) => { if (busy) event.preventDefault(); });
$('save').addEventListener('click', run(async () => {
  const currentIndex = profileIndex; const edits = changes();
  setBusy(true); $('save').disabled = true; $('closeReview').disabled = true; $('save').textContent = t('save.working');
  try {
    await layoutQueue;
    const result = await api('save', { token: connection.token, profileIndex: currentIndex, changes: edits });
    connection.snapshot = result.snapshot; drafts[currentIndex] = structuredClone(result.snapshot.profiles[currentIndex]); lastBackup = result.backup;
    message(() => t('save.done'));
  } catch (error) {
    if (error.backup) lastBackup = error.backup;
    // A failed write or stale snapshot must never be blindly retried.
    if (error.uncertain || error.status === 409 || !error.status) { connection = null; drafts = []; }
    throw error;
  } finally {
    $('reviewDialog').close(); $('save').disabled = false; $('closeReview').disabled = false; $('save').textContent = t('save.confirm'); setBusy(false);
  }
}));
async function getBackup(name) { const mappingName = name.replace('.json', '.gp2040'); const data = await api(`backups/${mappingName}`, undefined, 'GET'); download(data, `gp2040-mappings-${mappingName}`); }
$('backup').addEventListener('click', run(async () => { setBusy(true); try { const result = await api('backup', { token: connection.token }); lastBackup = result.name; await getBackup(lastBackup); message(() => t('backup.downloaded')); } finally { setBusy(false); } }));
$('downloadBackup').addEventListener('click', run(() => getBackup(lastBackup)));
$('exportLayout').addEventListener('click', () => download(layout, 'hitbox-layout.json'));
$('importLayout').addEventListener('click', () => $('layoutInput').click());
$('layoutInput').addEventListener('change', run(async () => {
  const file = $('layoutInput').files[0]; $('layoutInput').value = ''; if (!file) return;
  if (file.size > 16_000_000) throw new I18nError('layout.tooLarge');
  let parsed;
  try { parsed = JSON.parse(await file.text()); } catch { throw new I18nError('error.layoutImport'); }
  const imported = validateLayout(parsed);
  if (!await askConfirm(t('layout.importTitle'), t('layout.importBody'), t('layout.importAccept'))) return;
  layoutLoadFailed = false; layout = imported; selectedId = null; editSelection.clear(); swapFrom = null; render(); await persistLayout(); message(() => t('layout.imported'));
}));
$('help').addEventListener('click', () => $('helpDialog').showModal());
function renderLanguageSelector() {
  const languages = availableLanguages();
  const automatic = resolveLanguage(browserLanguages(), Object.fromEntries(languages.map((item) => [item.locale, item])));
  const options = [{ locale: 'auto', name: t('language.auto', languages.find((item) => item.locale === automatic)?.name || 'English') }, ...languages];
  $('language').replaceChildren(...options.map((item) => { const option = document.createElement('option'); option.value = item.locale; option.textContent = item.name; return option; }));
  $('language').value = options.some((item) => item.locale === languagePreference) ? languagePreference : currentLanguage();
}
function updateLanguage() {
  setLanguage(languagePreference === 'auto' ? browserLanguages() : [languagePreference]);
  translatePage(); renderLanguageSelector();
  if (!busy) $('save').textContent = t('save.confirm');
  if (connection) {
    for (const select of [$('profile'), $('copySource')]) for (const option of select.options) {
      const index = Number(option.value);
      option.textContent = `${profileName(index)}${drafts[index].enabled ? '' : t('profile.disabled')}`;
    }
  }
  message(lastMessage, lastMessageType); render();
}
$('language').addEventListener('change', () => {
  languagePreference = $('language').value;
  try { localStorage.setItem('hitbox.language', languagePreference); } catch {}
  updateLanguage();
});
window.addEventListener('languagechange', () => { if (languagePreference === 'auto' && !busy) updateLanguage(); });
renderLanguageSelector();
document.querySelectorAll('.menu-content button,.menu-content a').forEach((el) => el.addEventListener('click', () => { $('moreMenu').open = false; }));
document.addEventListener('click', (event) => { if (!$('moreMenu').contains(event.target)) $('moreMenu').open = false; });
document.addEventListener('keydown', (event) => { if (event.key === 'Escape') $('moreMenu').open = false; });
window.addEventListener('beforeunload', (event) => { if (allDirty() || layoutSaving || !$('layoutSaveError').hidden || busy) { event.preventDefault(); event.returnValue = ''; } });

try {
  const saved = await api('layout', undefined, 'GET');
  layoutSaved = Boolean(saved);
  if (saved) { layout = validateLayout(saved); try { if (layout.deviceKey) $('target').value = JSON.parse(layout.deviceKey)[0].replace('http://',''); } catch {} }
  $('localStatus').hidden = false;
  $('localStatus').textContent = saved ? t('layout.saved') : t('layout.autosave');
} catch (error) { layoutLoadFailed = true; message(() => t('layout.readError', errorText(error)), 'error'); }
render();
if (languageWarnings().length && !lastMessage) message(() => t('language.invalidFiles', languageWarnings().join(', ')), 'warning');
