# Cmd Ninja zsh integration.
# Load with:  eval "$(ninja init zsh)"
#
# Binds a ZLE widget: type a plain-English request at the prompt, press
# the hotkey, and the binary prints the INFO/RISK block to the tty. If
# the verdict allows autofill, the real command replaces the buffer,
# ready for Enter; otherwise the buffer is cleared so a destructive
# command must be copied or retyped deliberately.

_ninja_widget() {
  emulate -L zsh
  [[ -z $BUFFER ]] && return
  local out verb cmd
  print -r --                              # drop below the prompt line
  out=$(NINJA_SHELL=zsh "${NINJA_BIN:-ninja}" translate --wire -- "$BUFFER" </dev/tty)
  if [[ $? -ne 0 || -z $out ]]; then
    zle reset-prompt
    return
  fi
  verb=${out%%$'\t'*}
  cmd=${out#*$'\t'}
  zle reset-prompt
  if [[ $verb == FILL ]]; then
    BUFFER=$cmd
    CURSOR=${#BUFFER}
  else
    BUFFER=""                              # SHOW/BLOCK: leave prompt empty on purpose
    CURSOR=0
  fi
}
zle -N _ninja_widget
bindkey '%HOTKEY%' _ninja_widget
