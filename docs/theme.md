# TmuxAtlas Theme Palette Reference

To create a new theme, provide values for **two sections**: 31 CSS variables and 21 terminal colors. Add the theme as a new entry in the `themePresets` object in `web/src/theme.ts`.

## Section 1: CSS Variables (oklch format)

These control the entire UI. Format: `oklch(lightness chroma hue)`.

### Core

| Slot | Role |
|------|------|
| `--background` | Page/app background |
| `--foreground` | Default text color |
| `--card` | Card/panel background |
| `--card-foreground` | Card text |
| `--popover` | Popover/dropdown background |
| `--popover-foreground` | Popover text |
| `--primary` | Primary accent (buttons, links, active states) |
| `--primary-foreground` | Text on primary |
| `--secondary` | Secondary surfaces (hover backgrounds) |
| `--secondary-foreground` | Text on secondary |
| `--muted` | Subdued backgrounds (also used for empty sparkline bars) |
| `--muted-foreground` | Subdued text (labels, hints, fallback colors) |
| `--accent` | Highlight accent |
| `--accent-foreground` | Text on accent |
| `--destructive` | Error/danger color (also used for status errors, high usage bars) |
| `--destructive-foreground` | Text on destructive |
| `--border` | Borders, dividers, scrollbar thumb |
| `--input` | Input field backgrounds |
| `--ring` | Focus rings |
| `--success` | Success/active status indicators |
| `--warning` | Warning/waiting status indicators |

### Chart

| Slot | Role |
|------|------|
| `--chart-primary` | Sparkline bars, CPU usage bar, primary data viz color |
| `--chart-secondary` | Memory usage bar, secondary data viz color |

### Sidebar

| Slot | Role |
|------|------|
| `--sidebar` | Sidebar background |
| `--sidebar-foreground` | Sidebar text |
| `--sidebar-primary` | Sidebar active/selected |
| `--sidebar-primary-foreground` | Text on sidebar primary |
| `--sidebar-accent` | Sidebar hover |
| `--sidebar-accent-foreground` | Text on sidebar accent |
| `--sidebar-border` | Sidebar borders |
| `--sidebar-ring` | Sidebar focus rings |

## Section 2: Terminal Colors (hex format)

Standard ANSI terminal palette for xterm.js.

| Slot | Role |
|------|------|
| `background` | Terminal background |
| `foreground` | Terminal default text |
| `cursor` | Cursor color |
| `cursorAccent` | Text behind cursor |
| `selectionBackground` | Selection highlight (rgba) |
| `black` | ANSI black |
| `red` | ANSI red |
| `green` | ANSI green |
| `yellow` | ANSI yellow |
| `blue` | ANSI blue |
| `magenta` | ANSI magenta |
| `cyan` | ANSI cyan |
| `white` | ANSI white |
| `brightBlack` | Bright black (gray) |
| `brightRed` | Bright red |
| `brightGreen` | Bright green |
| `brightYellow` | Bright yellow |
| `brightBlue` | Bright blue |
| `brightMagenta` | Bright magenta |
| `brightCyan` | Bright cyan |
| `brightWhite` | Bright white |

## Section 3: Fixed Colors (not per-theme)

### Tool Brand Colors

These identify AI tools and stay constant across themes.

| Tool | Hex |
|------|-----|
| Claude | `#c4a0ff` |
| Codex | `#66e088` |
| Copilot | `#66b3ff` |
| OpenCode | `#bc8cff` |

### Status Colors

Status colors are derived from theme CSS variables:

| Status | CSS Variable |
|--------|-------------|
| Active / Completed | `var(--success)` |
| Waiting | `var(--warning)` |
| Error | `var(--destructive)` |

## Summary

**Total slots for a new theme: 31 CSS vars + 21 xterm colors = 52 values.**

Existing themes: `retro-blue` (default), `dark`, `light`, `green-phosphor`, `midnight`.

File to edit: `web/src/theme.ts`
