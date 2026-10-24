// Shared, cached application state.
import * as api from './api.js';

export const state = {
  settings: null,
  live: null,
  timers: null,
  aways: [],
  tagNames: [],
  tagCombos: [],
  recentTags: [],
  tagList: [],
  version: 0,
};

const listeners = new Set();
export function onChange(fn) { listeners.add(fn); return () => listeners.delete(fn); }
export function emit(what) { listeners.forEach((fn) => fn(what)); }

export async function loadTags() {
  const r = await api.get('tags/all');
  state.tagList = r.tags || [];
  state.tagCombos = state.tagList.map((x) => x.tags);
  state.tagNames = r.names || [];
  state.recentTags = r.recent || [];
  emit('tags');
  return r;
}

export function tagColor(tags) {
  const t = state.tagList.find((x) => x.tags === tags);
  return t && t.color ? '#' + t.color : null;
}

export async function refreshLive() {
  const r = await api.get('live');
  state.live = r.live;
  state.timers = r.timers;
  state.aways = r.aways || [];
  state.version = r.version;
  emit('live');
  return r;
}
