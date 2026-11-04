# Voice input from the browser

Programs on the server that capture from the default audio device, such as Claude Code `/voice`, need a microphone, and a remote server has none. With `TMUXATLAS_VOICE=on`, TmuxAtlas gives the server a virtual microphone fed by the browser.

```
browser mic → /ws/mic → pacat → PulseAudio null sink "atlas_mic" → monitor source → ALSA pulse plugin → SoX
```

## Setup

```bash
sudo apt-get install -y sox pulseaudio pulseaudio-utils libasound2-plugins
```

Set `TMUXATLAS_VOICE=on` for the TmuxAtlas service and restart it. At startup, and for each microphone stream, TmuxAtlas:

- starts PulseAudio for its user if needed
- creates the `atlas_mic` null sink and makes its monitor the default source
- unloads `module-suspend-on-idle`, which would clip the start of dictation
- creates `~/.asoundrc` so ALSA clients use PulseAudio; an existing file that routes elsewhere is left alone and reported as an error in the Terminal toolbar

Run the agent as the same user as TmuxAtlas, since PulseAudio and `~/.asoundrc` are per user.

## Usage

1. Open a session and click **Mic** in the Terminal toolbar. The browser asks for permission once.
2. In Claude Code, run `/voice`, then hold space to talk. Turn **Mic** on first: the recorder needs about a second to start on an idle source.
3. Click **● Mic on** to stop.

## Limits

- **Local sessions only.** The button appears only for sessions on the machine serving the page. Sessions on other hosts are not relayed.
- **Secure context.** Browsers allow the microphone only over HTTPS or on `localhost`.
- **One stream per host.** The newest tab takes over and the previous one stops.
- **No hold-to-talk on touch screens**, since it needs a held physical key.

Audio is forwarded in real time and never stored. Nothing is captured while the button is off.

## Troubleshooting

| Symptom | Check |
|---|---|
| `/voice` cannot open an audio device | `pactl info` shows `Default Source: atlas_mic.monitor`, and `~/.asoundrc` contains `pcm.!default { type pulse }` |
| No **Mic** button | `curl localhost:7654/api/voice` returns `"enabled":true`, and the session is on the local host |
| Recording is silent | `rec -q test.wav trim 0 3` while **Mic** is on, then `sox test.wav -n stat` |
| Wrong language | Change the dictation language in Claude Code with `/config` |
