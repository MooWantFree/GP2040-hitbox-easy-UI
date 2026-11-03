// A joystick is one selectable shape, with four independent input bindings.
export const shapeKey = (button) => button.groupId ? `group:${button.groupId}` : `button:${button.id}`;
export const selectionMembers = (buttons, keys) => buttons.filter((button) => keys.has(shapeKey(button)));
export function selectionShapes(buttons, keys) {
  return [...new Map(selectionMembers(buttons, keys).map((button) => [shapeKey(button), button])).values()];
}

export function scaleRange(shapes) {
  return {
    min: Math.ceil(Math.max(...shapes.map((b) => (b.shape === 'joystick' ? 12 : 2) / b.size * 100))),
    max: Math.floor(Math.min(...shapes.map((b) => (b.shape === 'joystick' ? 40 : 20) / b.size * 100))),
  };
}

// Clamp a shared delta so shapes keep their spacing when any one hits an edge.
export function movementDelta(shapes, dx, dy, aspectRatio) {
  if (!shapes.length) return { x: 0, y: 0 };
  let minX = -Infinity, maxX = Infinity, minY = -Infinity, maxY = Infinity;
  for (const button of shapes) {
    const halfWidth = button.size / 2;
    const halfHeight = halfWidth * aspectRatio / (button.shape === 'capsule' ? 2.4 : 1);
    minX = Math.max(minX, halfWidth - button.x);
    maxX = Math.min(maxX, 100 - halfWidth - button.x);
    minY = Math.max(minY, halfHeight - button.y);
    maxY = Math.min(maxY, 100 - halfHeight - button.y);
  }
  return {
    x: minX > maxX ? 0 : Math.max(minX, Math.min(maxX, dx)),
    y: minY > maxY ? 0 : Math.max(minY, Math.min(maxY, dy)),
  };
}
