import defaultSettings from '@/config/settings.json';
import type { AppState, LayoutMode, TabMode } from './types';

const layouts: LayoutMode[] = ['left', 'top', 'columns'];
const tabModes: TabMode[] = ['googlecard', 'card', 'card-gutter', 'rounded'];

// Merge new defaults into settings saved by earlier admin versions.
export function restoreSettings(serialized: string | null): AppState {
  let saved: Partial<AppState> = {};
  try {
    const value = serialized ? JSON.parse(serialized) : null;
    if (value && typeof value === 'object' && !Array.isArray(value)) saved = value;
  } catch {
    // A malformed browser preference must not prevent the admin from opening.
  }
  const layout = layouts.includes(saved.layout as LayoutMode)
    ? (saved.layout as LayoutMode)
    : saved.topMenu ? 'top' : 'left';
  const tabMode = tabModes.includes(saved.tabMode as TabMode)
    ? (saved.tabMode as TabMode)
    : 'googlecard';
  return {
    ...defaultSettings,
    ...saved,
    layout,
    topMenu: layout === 'top',
    tabMode,
    // Menus and routes belong to the current authenticated session.
    serverMenu: [],
    serverRoute: [],
    isDynamicAddedRoute: false,
    ...(layout === 'columns' ? { menuCollapse: false } : {}),
  } as AppState;
}
