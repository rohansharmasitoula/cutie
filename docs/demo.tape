# Records docs/demo.gif with https://github.com/charmbracelet/vhs: vhs docs/demo.tape
# (NO_COLOR or CI in the environment turn colors off: env -u NO_COLOR -u CI vhs docs/demo.tape)
Output docs/demo.gif
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
Type "CUTIE_DEMO=1 cutie" Enter
Sleep 1.5s
Show
Sleep 2s
Down Sleep 4s
Down Sleep 1.5s
Down Sleep 1.2s
Tab Sleep 1.5s
Down Sleep 1s
Down Sleep 2.5s
Tab Sleep 1.5s
Type "/" Sleep 300ms Type "billing" Sleep 1.2s Enter Sleep 1.5s
Escape Sleep 800ms
Type "?" Sleep 2.5s
Type "?" Sleep 800ms
Down Sleep 600ms
Type "x" Sleep 2s
Escape Sleep 1.5s
Type "q"
