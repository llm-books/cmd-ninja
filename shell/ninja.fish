# Cmd Ninja fish integration.
# Load with:  ninja init fish | source

function _ninja_widget
  set -l buf (commandline)
  test -z "$buf"; and return
  set -l bin ninja
  set -q NINJA_BIN; and set bin $NINJA_BIN
  echo >/dev/tty
  set -l out (env NINJA_SHELL=fish "$bin" translate --wire -- "$buf" </dev/tty)
  test -z "$out"; and commandline -f repaint; and return
  set -l parts (string split -m1 \t -- "$out")
  commandline -f repaint
  if test "$parts[1]" = FILL
    commandline -r "$parts[2]"
  else
    commandline -r ""                      # SHOW/BLOCK: copy or type it yourself
  end
end
bind %HOTKEY% _ninja_widget
