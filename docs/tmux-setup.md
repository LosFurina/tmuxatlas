# tmux Setup Guide

This guide covers recommended tmux configuration, plugins, and system setup for getting the most out of TmuxAtlas.

## Recommended tmux Configuration

Add these to your `~/.tmux.conf`:

```tmux
# --- Basics ---

# Use a modern terminal with true color and italic support
set -g default-terminal "tmux-256color"
set -ag terminal-overrides ",xterm-256color:RGB"

# Increase scrollback buffer (default 2000)
set -g history-limit 50000

# Reduce escape delay (important for vim/neovim users)
set -sg escape-time 10

# Enable focus events (useful for vim autoread, etc.)
set -g focus-events on

# Start window/pane numbering at 1 (easier to reach on keyboard)
set -g base-index 1
setw -g pane-base-index 1

# Renumber windows when one is closed
set -g renumber-windows on

# Enable mouse support (scrolling, pane selection, resizing)
set -g mouse on

# --- Clipboard (required for tmuxatlas) ---

# Let tmux process OSC 52 clipboard sequences
set -g set-clipboard on

# Allow applications to send escape sequences through tmux
# (needed for clipboard sync, image protocols, etc.)
set -g allow-passthrough on

# --- Display ---

# Show longer status messages
set -g display-time 2000

# Refresh status bar more frequently
set -g status-interval 5

# Activity monitoring
setw -g monitor-activity on
set -g visual-activity off
```

### Keybinding Suggestions

These are optional quality-of-life improvements:

```tmux
# Remap prefix to Ctrl-a (easier to reach than Ctrl-b)
# unbind C-b
# set -g prefix C-a
# bind C-a send-prefix

# Split panes with | and -
bind | split-window -h -c "#{pane_current_path}"
bind - split-window -v -c "#{pane_current_path}"

# New windows open in the current path
bind c new-window -c "#{pane_current_path}"

# Reload config
bind r source-file ~/.tmux.conf \; display-message "Config reloaded"

# Vim-style pane navigation
bind h select-pane -L
bind j select-pane -D
bind k select-pane -U
bind l select-pane -R

# Resize panes with Shift+arrow
bind -r H resize-pane -L 5
bind -r J resize-pane -D 5
bind -r K resize-pane -U 5
bind -r L resize-pane -R 5
```

## Recommended Plugins

[TPM (Tmux Plugin Manager)](https://github.com/tmux-plugins/tpm) is the standard way to manage tmux plugins.

### Installing TPM

```bash
git clone https://github.com/tmux-plugins/tpm ~/.tmux/plugins/tpm
```

Add to the bottom of `~/.tmux.conf`:

```tmux
# List of plugins
set -g @plugin 'tmux-plugins/tpm'
set -g @plugin 'tmux-plugins/tmux-sensible'

# Initialize TPM (keep this line at the very bottom)
run '~/.tmux/plugins/tpm/tpm'
```

Press `prefix + I` (capital I) inside tmux to install plugins.

### Recommended Plugins

| Plugin | What it does |
|--------|-------------|
| [tmux-sensible](https://github.com/tmux-plugins/tmux-sensible) | Sensible defaults everyone can agree on |
| [tmux-resurrect](https://github.com/tmux-plugins/tmux-resurrect) | Persist sessions across system restarts — saves/restores pane layout, working directories, and running programs |
| [tmux-continuum](https://github.com/tmux-plugins/tmux-continuum) | Auto-saves resurrect sessions every 15 minutes, auto-restores on tmux start |
| [tmux-yank](https://github.com/tmux-plugins/tmux-yank) | Copy to system clipboard from tmux copy mode |
| [tmux-fingers](https://github.com/Morantron/tmux-fingers) | Quick copy — highlights URLs, file paths, hashes, IPs on screen for fast selection |
| [tmux-fzf](https://github.com/sainnhe/tmux-fzf) | Fuzzy finder for sessions, windows, panes, commands |

Example plugin block:

```tmux
set -g @plugin 'tmux-plugins/tpm'
set -g @plugin 'tmux-plugins/tmux-sensible'
set -g @plugin 'tmux-plugins/tmux-resurrect'
set -g @plugin 'tmux-plugins/tmux-continuum'
set -g @plugin 'tmux-plugins/tmux-yank'

# Resurrect settings
set -g @resurrect-capture-pane-contents 'on'
set -g @resurrect-strategy-nvim 'session'  # if using neovim

# Continuum settings
set -g @continuum-restore 'on'       # auto-restore on tmux start
set -g @continuum-save-interval '15' # save every 15 minutes

run '~/.tmux/plugins/tpm/tpm'
```

### A Note on tmux-resurrect + TmuxAtlas

`tmux-resurrect` and `tmux-continuum` are especially useful with TmuxAtlas. Since TmuxAtlas monitors tmux sessions, having them survive reboots means your TmuxAtlas dashboard stays populated without manual session recreation. Combined with the systemd service below, your tmux sessions (and TmuxAtlas) survive reboots automatically.

## systemd User Service

A systemd user service ensures the tmux server starts automatically when you log in (or at boot with lingering enabled), so your sessions and TmuxAtlas are always available.

### tmux Server Service

Create `~/.config/systemd/user/tmux-server.service`:

```ini
[Unit]
Description=tmux server
Documentation=man:tmux(1)

[Service]
Type=forking
ExecStart=/usr/bin/tmux new-session -d -s main
ExecStop=/usr/bin/tmux kill-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

### TmuxAtlas Server Service

Create `~/.config/systemd/user/tmuxatlas.service`:

```ini
[Unit]
Description=TmuxAtlas web dashboard for tmux
After=tmux-server.service
Requires=tmux-server.service

[Service]
Type=simple
ExecStart=%h/.local/bin/tmuxatlas server
Restart=on-failure
RestartSec=5
Environment=TMUXATLAS_LISTEN=127.0.0.1:7654

# Uncomment and customize as needed:
# Environment=TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com
# Environment=TMUXATLAS_HUB=https://tmuxatlas.example.com
# Environment=TMUXATLAS_LOCAL_ONLY=true

[Install]
WantedBy=default.target
```

Adjust the `ExecStart` path to wherever your `tmuxatlas` binary is installed.

### Enable and Start

```bash
# Reload systemd user daemon
systemctl --user daemon-reload

# Enable both services (start on login)
systemctl --user enable tmux-server.service
systemctl --user enable tmuxatlas.service

# Start them now
systemctl --user start tmux-server.service
systemctl --user start tmuxatlas.service

# Check status
systemctl --user status tmux-server.service
systemctl --user status tmuxatlas.service
```

### Enable Lingering (Start at Boot Without Login)

By default, systemd user services only run while the user has an active login session. To keep tmux and TmuxAtlas running at boot (even before you log in), enable lingering:

```bash
sudo loginctl enable-linger $USER
```

This is particularly important for headless servers or remote machines where you want TmuxAtlas to be available immediately after a reboot.

### Logs

```bash
# View tmuxatlas logs
journalctl --user -u tmuxatlas.service -f

# View tmux server logs
journalctl --user -u tmux-server.service -f
```
