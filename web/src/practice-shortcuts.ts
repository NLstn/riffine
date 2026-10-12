// Foreground keyboard discovery/focus policy belongs to the client. Actions
// below call the same shared-core facade as the visible playback buttons.
export const shortcutActions = ['play_pause', 'restart', 'stop', 'rewind_phrase', 'seek_backward', 'seek_forward', 'set_a', 'set_b', 'next_region', 'speed_down', 'speed_up', 'practice_reset'] as const;
export type ShortcutAction = typeof shortcutActions[number];
export interface ShortcutSettings { version: 1; enabled: boolean; bindings: Record<ShortcutAction, string> }
export const defaultShortcuts: ShortcutSettings = { version: 1, enabled: true, bindings: {play_pause:'Space', restart:'R', stop:'S', rewind_phrase:'Q', seek_backward:'J', seek_forward:'L', set_a:'A', set_b:'B', next_region:'N', speed_down:'D', speed_up:'F', practice_reset:'T'} };
const preferenceKey = 'riffine.practice.shortcuts.v1';
export const shortcutKeys = ['', 'Space', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];
export function validateShortcuts(value: unknown): ShortcutSettings {
 if (!value || typeof value !== 'object') throw new Error('Shortcut settings are invalid.');
 const settings = value as Partial<ShortcutSettings>;
 if (settings.version !== 1 || typeof settings.enabled !== 'boolean' || !settings.bindings || typeof settings.bindings !== 'object') throw new Error('Shortcut settings are invalid.');
 const bindings = {} as Record<ShortcutAction, string>;
 const used = new Set<string>();
 for (const action of shortcutActions) {
  // Additive v1 migration keeps the original three required bindings and
  // leaves new actions unbound for existing preferences, without a write.
  const key = settings.bindings[action] ?? (['play_pause','restart','stop'].includes(action) || Object.prototype.hasOwnProperty.call(settings.bindings,action) ? undefined : '');
  if (!shortcutKeys.includes(key)) throw new Error('Choose Space or a letter key for each shortcut.');
  if (key && used.has(key)) throw new Error('Each shortcut must use a different key.');
  if (key) used.add(key); bindings[action] = key;
 }
 return {version:1, enabled:settings.enabled, bindings};
}
export function loadShortcuts(): {settings: ShortcutSettings; warning?: string} {
 try {
  const stored = localStorage.getItem(preferenceKey);
  if (!stored) return {settings:validateShortcuts(defaultShortcuts)};
  if (stored.length > 4096) throw new Error('Stored shortcut settings exceed the limit.');
  return {settings:validateShortcuts(JSON.parse(stored))};
 } catch { return {settings:validateShortcuts(defaultShortcuts), warning:'Saved shortcuts could not load. Defaults are active; save changes to keep your preferred keys.'}; }
}
export function saveShortcuts(settings: ShortcutSettings): ShortcutSettings {
 const accepted = validateShortcuts(settings);
 try { localStorage.setItem(preferenceKey, JSON.stringify(accepted)); }
 catch { throw new Error('Shortcuts could not be saved on this device. Your current shortcuts are unchanged.'); }
 return accepted;
}
interface KeyInput { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean; isComposing: boolean; target: EventTarget | null }
export function matchShortcut(event: KeyInput, settings: ShortcutSettings): ShortcutAction | undefined {
 if (!settings.enabled || event.ctrlKey || event.metaKey || event.altKey || event.shiftKey || event.isComposing) return;
 const target = event.target instanceof Element ? event.target : undefined;
 // Ancestor lookup covers nested editable content and custom accessible fields.
 if (target?.closest('input,textarea,select,[role="textbox"],[role="combobox"],[contenteditable]:not([contenteditable="false"])')) return;
 const key = event.key === ' ' ? 'Space' : /^[a-z]$/i.test(event.key) ? event.key.toUpperCase() : '';
 // Keep native Space activation on focused controls, including nested labels.
 if (!key || (key === 'Space' && target?.closest('button,summary,a[href],[role="button"],[role="checkbox"],[role="radio"],[role="switch"],[role="tab"]'))) return;
 return shortcutActions.find(action => settings.bindings[action] === key);
}
