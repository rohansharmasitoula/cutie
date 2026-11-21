# Records docs/demo.gif with https://github.com/charmbracelet/vhs, from the repo root:
#   env -u NO_COLOR -u CI vhs docs/demo.tape
# (NO_COLOR or CI in the environment turn colors off.) Needs herdr. It builds this
# checkout, opens a throwaway herdr session (cutie-demo, with docs/herdr-demo.toml) and
# runs CUTIE_DEMO=herdr there: made-up PRs, real herdr panes, scripted agents, no AI.
Output "docs/demo.gif"
Set Shell "bash"
Set FontSize 13
Set Width 1600
Set Height 900
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env BASH_SILENCE_DEPRECATION_WARNING "1"
Env PS1 "$ "

Hide
Type `unset HERDR_ENV HERDR_PANE_ID HERDR_TAB_ID HERDR_WORKSPACE_ID HERDR_SOCKET_PATH; export HERDR_CONFIG_PATH="$PWD/docs/herdr-demo.toml"` Enter
Type "go build -o /tmp/cutie-demo/cutie . && (herdr session stop cutie-demo; herdr session delete cutie-demo) >/dev/null 2>&1; mkdir -p /tmp/cutie-rec/cutie && cd /tmp/cutie-rec/cutie && clear && herdr --session cutie-demo" Enter
Sleep 4s
Type "clear; CUTIE_DEMO=herdr /tmp/cutie-demo/cutie" Enter
Wait+Screen@60s /Agents · 4/
Sleep 2s
Show

# Mine: a PR that needs work, with the agent fixing it.
Sleep 1.5s
Down Sleep 3s
# enter jumps to the agent's herdr pane; ctrl+b 1 is back to cutie.
Enter Sleep 4s
Ctrl+B Type "1" Sleep 1.5s

# Review: #409's agent drafted its review on copilot and waits for an OK.
Tab Sleep 1s
Down Sleep 2.5s
# Queue #240 behind it and put #240 on autopilot.
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 1.2s
Type "a" Sleep 2.5s
Type "m" Sleep 2.5s
# Approve #409's review in its pane...
Up Sleep 300ms
Up Sleep 300ms
Up Sleep 300ms
Up Sleep 1s
Enter Sleep 3.5s
Type "y" Enter Sleep 4s
# ...and the queue starts #240's review by itself.
Ctrl+B Type "1" Sleep 3s
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 4s
Enter Sleep 4s
Ctrl+B Type "1" Sleep 1.5s

# Issues: start an agent on one with a single key, then enter again jumps to it.
Tab Sleep 1s
Down Sleep 2s
Down Sleep 300ms
Down Sleep 1s
Enter Sleep 3s
Enter Sleep 4s
Ctrl+B Type "1" Sleep 2s
Type "?" Sleep 3s
Type "?" Sleep 1s

Hide
Ctrl+B
Type "q"
Sleep 1s
Type "herdr session stop cutie-demo; herdr session delete cutie-demo" Enter
Sleep 2s
