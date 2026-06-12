# Cmd Ninja bash integration.
# Load with:  eval "$(ninja init bash)"

_ninja_widget() {
  [[ -z $READLINE_LINE ]] && return
  local out verb cmd
  printf '\n' >/dev/tty
  out=$(NINJA_SHELL=bash "${NINJA_BIN:-ninja}" translate --wire -- "$READLINE_LINE" </dev/tty) || return
  [[ -z $out ]] && return
  verb=${out%%$'\t'*}
  cmd=${out#*$'\t'}
  if [[ $verb == FILL ]]; then
    READLINE_LINE=$cmd
    READLINE_POINT=${#READLINE_LINE}
  else
    READLINE_LINE=""                       # SHOW/BLOCK: copy or type it yourself
    READLINE_POINT=0
  fi
}
bind -x '"%HOTKEY%": _ninja_widget'
