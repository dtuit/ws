package command

import "fmt"

const CompletionCommandFallbackSentinel = "__ws_complete_commands__"

// ShellInitScript returns the shell integration and completion script for ws.
func ShellInitScript() string {
	return fmt.Sprintf(`_ws_wants_help() {
  # Returns 0 if any arg is -h / --help / help (so output is not a dir+cmd pair).
  local a
  for a in "$@"; do
    case "$a" in
      -h|--help|help) return 0 ;;
    esac
  done
  return 1
}
ws() {
  case "$1" in
    cd)
      if _ws_wants_help "${@:2}"; then
        command ws "$@"
        return
      fi
      local dir
      dir="$(command ws cd "${@:2}")" && cd "$dir"
      ;;
    agent)
      # Subcommands that don't follow the dir+cmd stdout protocol pass through.
      if _ws_wants_help "${@:2}"; then
        command ws "$@"
        return
      fi
      case "$2" in
        ls|list|pin|unpin)
          command ws "$@"
          ;;
        *)
          local _ws_agent_info _ws_agent_dir _ws_agent_cmd
          _ws_agent_info="$(command ws agent "${@:2}")"
          _ws_agent_dir="$(printf '%%s\n' "$_ws_agent_info" | head -1)"
          _ws_agent_cmd="$(printf '%%s\n' "$_ws_agent_info" | tail -n +2)"
          cd "$_ws_agent_dir" && eval "$_ws_agent_cmd"
          ;;
      esac
      ;;
    *)
      command ws "$@"
      ;;
  esac
}
_ws_delegate_bash() {
  local start actual_start cmd cur prev spec func cmdspec line
  start="${1:-0}"
  actual_start=$((start + 1))
  if [ "$actual_start" -gt "${#COMP_WORDS[@]}" ]; then
    return 0
  fi
  cmd="${COMP_WORDS[$actual_start]}"
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev=""
  if [ "$COMP_CWORD" -gt "$actual_start" ]; then
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  spec="$(complete -p "$cmd" 2>/dev/null)" || {
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
    return 0
  }

  local -a old_comp_words
  local old_comp_cword old_comp_line old_comp_point old_comp_type old_comp_key
  old_comp_words=( "${COMP_WORDS[@]}" )
  old_comp_cword=$COMP_CWORD
  old_comp_line="${COMP_LINE:-}"
  old_comp_point="${COMP_POINT:-0}"
  old_comp_type="${COMP_TYPE:-}"
  old_comp_key="${COMP_KEY:-}"

  COMP_WORDS=( "${old_comp_words[@]:$actual_start}" )
  COMP_CWORD=$((old_comp_cword - actual_start))
  COMP_LINE="${COMP_WORDS[*]}"
  COMP_POINT=${#COMP_LINE}

  if [[ "$spec" =~ [[:space:]]-F[[:space:]]+([^[:space:]]+) ]]; then
    func="${BASH_REMATCH[1]}"
    COMPREPLY=()
    "$func" "$cmd" "$cur" "$prev"
  elif [[ "$spec" =~ [[:space:]]-C[[:space:]]+([^[:space:]]+) ]]; then
    cmdspec="${BASH_REMATCH[1]}"
    COMPREPLY=( $(command "$cmdspec" "$cmd" "$cur" "$prev") )
  else
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
  fi

  COMP_WORDS=( "${old_comp_words[@]}" )
  COMP_CWORD=$old_comp_cword
  COMP_LINE="$old_comp_line"
  COMP_POINT=$old_comp_point
  COMP_TYPE="$old_comp_type"
  COMP_KEY="$old_comp_key"
}

_ws_complete_bash() {
  local cur prev delegate_start line value
  local -a completions
  COMPREPLY=()
  completions=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev=""
  if [ "${COMP_CWORD}" -gt 0 ]; then
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  if [ "$prev" = "-w" ] || [ "$prev" = "--workspace" ]; then
    COMPREPLY=( $(compgen -d -- "$cur") )
    return 0
  fi
  while IFS= read -r line; do
    completions+=( "$line" )
  done < <(command ws __complete "$((COMP_CWORD-1))" "${COMP_WORDS[@]:1}")
  if [ "${#completions[@]}" -eq 1 ] && [[ "${completions[0]}" == "%s":* ]]; then
    delegate_start="${completions[0]#%s:}"
    _ws_delegate_bash "$delegate_start"
    return 0
  fi
  if [ "${#completions[@]}" -eq 1 ] && [ "${completions[0]}" = "%s" ]; then
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
    return 0
  fi
  # Each completion line is "<group>\t<value>\t<desc>"; bash only renders
  # values, so strip to the second field.
  for line in "${completions[@]}"; do
    value="${line#*$'\t'}"
    value="${value%%$'\t'*}"
    COMPREPLY+=( "$value" )
  done
}

_ws_delegate_zsh() {
  local start actual_start old_current
  local -a old_words
  start="${1:-0}"
  actual_start=$((start + 2))
  old_current=$CURRENT
  old_words=("${words[@]}")
  words=("${old_words[@]:$actual_start}")
  CURRENT=$((old_current - actual_start + 1))
  _normal
  words=("${old_words[@]}")
  CURRENT=$old_current
}

_ws_complete_zsh() {
  local prev delegate_start
  local -a ws_words completions
  if (( CURRENT > 1 )); then
    prev="${words[CURRENT-1]}"
  else
    prev=""
  fi
  if [[ "$prev" == "-w" || "$prev" == "--workspace" ]]; then
    _files -/
    return
  fi
  ws_words=()
  local i
  for (( i = 2; i <= ${#words}; i++ )); do
    ws_words+=("${words[i]}")
  done
  completions=("${(@f)$(command ws __complete "$((CURRENT-2))" "${ws_words[@]}")}")
  if (( ${#completions[@]} == 1 )) && [[ "${completions[1]}" == "%s":* ]]; then
    delegate_start="${completions[1]#%s:}"
    _ws_delegate_zsh "$delegate_start"
    return
  fi
  if (( ${#completions[@]} == 1 )) && [[ "${completions[1]}" == "%s" ]]; then
    _command_names
    return
  fi
  if (( ${#completions[@]} == 0 )); then
    return
  fi

  # Each line is "<group>\t<value>\t<desc>". Bucket by group, then call
  # _describe per group so zsh renders headings + descriptions. Empty group
  # falls into a no-heading bucket rendered last via plain compadd.
  local -A _ws_buckets
  local -a _ws_order
  local line group value desc rest
  for line in "${completions[@]}"; do
    group="${line%%$'\t'*}"
    rest="${line#*$'\t'}"
    value="${rest%%$'\t'*}"
    desc="${rest#*$'\t'}"
    [[ "$desc" == "$value" ]] && desc=""

    if [[ -z "${_ws_buckets[$group]+x}" ]]; then
      _ws_order+=("$group")
    fi
    if [[ -n "$desc" ]]; then
      _ws_buckets[$group]+="${value}"$'\x1f'"${desc}"$'\n'
    else
      _ws_buckets[$group]+="${value}"$'\x1f'$'\n'
    fi
  done

  for group in "${_ws_order[@]}"; do
    local -a _ws_items _ws_pairs
    _ws_items=()
    _ws_pairs=()
    local entry v d
    while IFS= read -r entry; do
      [[ -z "$entry" ]] && continue
      v="${entry%%$'\x1f'*}"
      d="${entry#*$'\x1f'}"
      [[ "$d" == "$entry" ]] && d=""
      _ws_items+=("$v")
      if [[ -n "$d" ]]; then
        _ws_pairs+=("$v:$d")
      else
        _ws_pairs+=("$v")
      fi
    done <<< "${_ws_buckets[$group]}"

    if [[ -z "$group" ]]; then
      compadd -- "${_ws_items[@]}"
    else
      _describe -t "$group" "$group" _ws_pairs
    fi
  done
}

if [ -n "${BASH_VERSION:-}" ]; then
  complete -F _ws_complete_bash ws
elif [ -n "${ZSH_VERSION:-}" ]; then
  if ! whence compdef >/dev/null 2>&1; then
    autoload -Uz compinit
    compinit
  fi
  compdef _ws_complete_zsh ws
fi
`, CompletionCommandFallbackSentinel, CompletionCommandFallbackSentinel, CompletionCommandFallbackSentinel, CompletionCommandFallbackSentinel, CompletionCommandFallbackSentinel, CompletionCommandFallbackSentinel)
}
