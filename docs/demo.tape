# Records docs/demo.gif with https://github.com/charmbracelet/vhs, from the repo root:
#   env -u NO_COLOR -u CI vhs docs/demo.tape
# (NO_COLOR or CI in the environment turn colors off.) It builds this checkout first,
# so the GIF always shows the current code, then runs it on demo data.
Output "docs/demo.gif"
Set Shell "bash"
Set FontSize 13
Set Width 1500
Set Height 820
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms
Env TERM "xterm-256color"
Env COLORTERM "truecolor"

Hide
Type "go build -o /tmp/cutie-demo/cutie . && clear && CUTIE_DEMO=1 /tmp/cutie-demo/cutie" Enter
Wait+Screen@60s /needs work/
Sleep 1s
Show
# Mine: a PR that needs work, its threads and the agent fixing it.
Sleep 1.5s
Down Sleep 3.5s
# A stacked pair: #421 builds on #418, whose agent is done.
Down Sleep 1.5s
Down Sleep 1.5s
# Watch #421: its agent wakes up by itself on new review feedback.
Type "w" Sleep 2s

# Review: a re-requested PR whose agent is on autopilot and waiting for you.
Tab Sleep 1.2s
Down Sleep 3s
# A fresh PR to review: queue it behind the running review, then put it on autopilot.
Down Sleep 400ms
Down Sleep 400ms
Down Sleep 400ms
Down Sleep 1.5s
Type "a" Sleep 2.5s
Type "m" Sleep 3s

# Issues: linked PRs, and an agent already building one.
Tab Sleep 1.2s
Down Sleep 3s
# Start an agent on another issue with one key.
Down Sleep 400ms
Down Sleep 1.2s
Enter Sleep 2.5s

# Back to Mine: filter, then the key help.
Type "1" Sleep 1.2s
Type "/" Sleep 300ms Type "billing" Sleep 1.2s Enter Sleep 1.5s
Escape Sleep 800ms
Type "?" Sleep 3s
Type "?" Sleep 800ms
Type "q"
