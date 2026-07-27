# Implementation baseline

Recorded before implementation on 2026-07-27.

## Reused capabilities

`improve-state-web-and-operations` already provides the implementation baseline for:

- revisioned application state, stable Host/Session selectors and guarded reconnect lifecycle (`3.2`–`3.6`);
- mobile Terminal key toolbar, Ctrl/Alt one-shot/locked input, generation-safe Clipboard, safe-area and Visual Viewport fitting (`7.1`–`7.4`);
- bundled fonts, Nerd symbol fallback, lazy xterm loading and bundle gates (`8.1`–`8.3`).

This change extends those paths. It does not create a second mobile toolbar, state transport or font loader. The prior change's browser/CI tasks `9.1`–`9.4` remain incomplete and overlap with this change's final E2E gate.

## Test and bundle baseline

- Vitest: 10 files, 23 tests passed.
- Production Web build: passed.
- Chromium E2E: 6 tests passed.
- Mobile WebKit E2E: 1 test passed after installing the matching Playwright WebKit runtime. The Playwright downloader stalled during extraction twice, so the verified archive was unpacked with the system archive tool into the expected cache path before the successful rerun.
- Entry gzip: 106,775 / 133,120 bytes.
- Xterm gzip: 82,154 / 87,040 bytes.
- Total Brotli: 161,232 / 179,200 bytes.

## Interaction migration inventory

### Shortcuts

- `App.tsx` owns Quick Switcher, Help, Overview, Sidebar, Settings, Lock and next-alert shortcuts.
- `Terminal.tsx` separately owns fullscreen and Escape behavior.
- `QuickSwitcher.tsx` owns overlay navigation and cancellation.
- `HelpModal.tsx` maintains a static shortcut list.
- `useTerminal.ts` separately filters terminal control keys, so application commands can execute while a control byte also reaches the PTY.

The command registry must replace these independent sources. Terminal-native `Ctrl+H` and `Ctrl+L` remain PTY input; application commands must explicitly opt in and suppress PTY delivery.

### Connection and feedback

- `StateConnectionNotice` describes canonical state connection.
- `TopBar` and `StatusBar` independently summarize connectivity.
- `Terminal` renders a generic disconnected overlay based on its own boolean.
- Runtime mutations and Preferences use separate error paths, and Preferences can keep an optimistic value after a failed response.

The Workspace status model must derive one presentation from Hub, Agent and PTY facts and use shared Toast/inline feedback.

### Preferences

- `PreferencesContext` is the server-confirmed preference source.
- App initialization currently reads URL and local brand storage directly for view/sidebar behavior.
- Sidebar also maintains hidden/session UI state separately.

URL targets keep highest priority; otherwise confirmed `default_view` and `sidebar_default` must initialize App and Sidebar. Browser-only Pin/recent data remains namespaced local storage.

### Terminal listeners and state

- `useTerminal` installs `mousedown`, `keydown` and an anonymous `contextmenu` handler during each connection without disposing all of them.
- `pendingClipboard` is module-global.
- `window.__term` exposes the production terminal instance.
- target/disconnect does not synchronously clear every presented PTY state.

The terminal lifecycle refactor must own every disposable/listener per generation, remove the debug global and keep Clipboard/Composer data target-safe.
