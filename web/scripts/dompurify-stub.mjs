const stub = {
  sanitize: (t) => (t == null ? '' : String(t)),
  addHook: () => {},
  removeHook: () => {},
  removeHooks: () => {},
  removeAllHooks: () => {},
  setConfig: () => {},
  clearConfig: () => {},
  isValidAttribute: () => true,
  version: 'stub',
  removed: [],
  isSupported: true,
}
export default stub
