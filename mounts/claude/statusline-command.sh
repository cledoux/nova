#!/bin/bash
# vim: set foldmarker={{{,}}} foldmethod=marker foldlevel=0 spell:
# Header {{{
#
#   Author: Charles LeDoux
#
#   Bashrc Prompt configuration. Needs to be loaded in ~/.bashrc
#
#   This file is in the public domain.
#
#   Suggestions and bugs can be emailed to charles@charlesledoux.com
# }}}

# Color Variables {{{
# Copied from ~/.bashrc.prompt.
# Printf doesn't use the surrounding "\[\]" echo needs.

# Reset
ColorOff="\033[0m"       # Text Reset


# Regular Colors
Black="\033[0;30m"        # Black
Red="\033[0;31m"          # Red
Green="\033[0;32m"        # Green
Yellow="\033[0;33m"       # Yellow
Blue="\033[0;34m"         # Blue
Purple="\033[0;35m"       # Purple
Cyan="\033[0;36m"         # Cyan
White="\033[0;37m"        # White

# Bold
BBlack="\033[1;30m"       # Black
BRed="\033[1;31m"         # Red
BGreen="\033[1;32m"       # Green
BYellow="\033[1;33m"      # Yellow
BBlue="\033[1;34m"        # Blue
BPurple="\033[1;35m"      # Purple
BCyan="\033[1;36m"        # Cyan
BWhite="\033[1;37m"       # White

# Underline
UBlack="\033[4;30m"       # Black
URed="\033[4;31m"         # Red
UGreen="\033[4;32m"       # Green
UYellow="\033[4;33m"      # Yellow
UBlue="\033[4;34m"        # Blue
UPurple="\033[4;35m"      # Purple
UCyan="\033[4;36m"        # Cyan
UWhite="\033[4;37m"       # White

# Background
On_Black="\033[40m"       # Black
On_Red="\033[41m"         # Red
On_Green="\033[42m"       # Green
On_Yellow="\033[43m"      # Yellow
On_Blue="\033[44m"        # Blue
On_Purple="\033[45m"      # Purple
On_Cyan="\033[46m"        # Cyan
On_White="\033[47m"       # White

# High Intensty
IBlack="\033[0;90m"       # Black
IRed="\033[0;91m"         # Red
IGreen="\033[0;92m"       # Green
IYellow="\033[0;93m"      # Yellow
IBlue="\033[0;94m"        # Blue
IPurple="\033[0;95m"      # Purple
ICyan="\033[0;96m"        # Cyan
IWhite="\033[0;97m"       # White

# Bold High Intensty
BIBlack="\033[1;90m"      # Black
BIRed="\033[1;91m"        # Red
BIGreen="\033[1;92m"      # Green
BIYellow="\033[1;93m"     # Yellow
BIBlue="\033[1;94m"       # Blue
BIPurple="\033[1;95m"     # Purple
BICyan="\033[1;96m"       # Cyan
BIWhite="\033[1;97m"      # White

# High Intensty backgrounds
On_IBlack="\033[0;100m"   # Black
On_IRed="\033[0;101m"     # Red
On_IGreen="\033[0;102m"   # Green
On_IYellow="\033[0;103m"  # Yellow
On_IBlue="\033[0;104m"    # Blue
On_IPurple="\033[10;95m"  # Purple
On_ICyan="\033[0;106m"    # Cyan
On_IWhite="\033[0;107m"   # White

# Various variables you might want for your PS1 prompt instead
Time12h="\T"
Time12a="\@"
PathShort="\w"
PathFull="\W"
NewLine="\n"
Jobs="\j"
# }}}

# Read JSON data that Claude Code sends to stdin
input=$(cat)

# Extract fields using jq. The "// fallback" snippets provide default values.
MODEL=$(echo "$input" | jq -r '.model.display_name')
CWD=$(echo "$input" | jq -r '.workspace.current_dir // .cwd')
CTX_PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
RATE_5H=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
RATE_7D=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')
SESSION_NAME=$(echo "$input" | jq -r '.session_name // empty')
OUTPUT_STYLE=$(echo "$input" | jq -r '.output_style.name // empty')
VIM_MODE=$(echo "$input" | jq -r '.vim.mode // empty')
AGENT_NAME=$(echo "$input" | jq -r '.agent.name // empty')
AGENT_TYPE=$(echo "$input" | jq -r '.agent.type // empty')
WORKTREE_NAME=$(echo "$input" | jq -r '.worktree.name // empty')
WORKTREE_BRANCH=$(echo "$input" | jq -r '.worktree.branch // empty')
SESSION_COST=$(echo "$input" | jq -r '.cost_usd // empty')
SESSION_DURATION_MS=$(echo "$input" | jq -r '.duration_ms // empty')

# Build progress bar: printf -v creates a run of spaces, then
# ${var// /▓} replaces each space with a block character
BAR_WIDTH=10

build_bar() {
  local pct="$1"
  local filled=$(( pct * BAR_WIDTH / 100 ))
  local empty=$(( BAR_WIDTH - filled ))
  local bar_color=""
  if [ "$pct" -ge 80 ]; then
    bar_color="$Red"
  elif [ "$pct" -ge 50 ]; then
    bar_color="$Yellow"
  else
    bar_color="$Green"
  fi
  local bar=""
  [ "$filled" -gt 0 ] && printf -v fill "%${filled}s" && bar="${bar_color}${fill// /▓}${ColorOff}"
  [ "$empty" -gt 0 ] && printf -v pad "%${empty}s" && bar="${bar}${pad// /░}"
  echo "$bar"
}

CTX_BAR=$(build_bar "$CTX_PCT")

# ── Line 1: Model + session name + agent + worktree ──────────────────────────
LINE1="${Green}${MODEL}${ColorOff}"

# Session name (set via /rename)
if [ -n "$SESSION_NAME" ]; then
  LINE1="${LINE1} ${BWhite}\"${SESSION_NAME}\"${ColorOff}"
fi

# Agent info (present when started with --agent)
if [ -n "$AGENT_NAME" ]; then
  AGENT_LABEL="$AGENT_NAME"
  [ -n "$AGENT_TYPE" ] && AGENT_LABEL="${AGENT_LABEL}/${AGENT_TYPE}"
  LINE1="${LINE1} ${BPurple}[agent:${AGENT_LABEL}]${ColorOff}"
fi

# Worktree info (present in --worktree sessions)
if [ -n "$WORKTREE_NAME" ]; then
  WT_LABEL="$WORKTREE_NAME"
  [ -n "$WORKTREE_BRANCH" ] && WT_LABEL="${WT_LABEL}:${WORKTREE_BRANCH}"
  LINE1="${LINE1} ${BCyan}[wt:${WT_LABEL}]${ColorOff}"
fi

# Output style (when not the default)
if [ -n "$OUTPUT_STYLE" ] && [ "$OUTPUT_STYLE" != "default" ]; then
  LINE1="${LINE1} ${IYellow}(${OUTPUT_STYLE})${ColorOff}"
fi

# Vim mode indicator
if [ -n "$VIM_MODE" ]; then
  case "$VIM_MODE" in
    INSERT) VIM_COLOR="$BGreen" ;;
    NORMAL) VIM_COLOR="$BYellow" ;;
    *)      VIM_COLOR="$BWhite" ;;
  esac
  LINE1="${LINE1} ${VIM_COLOR}[${VIM_MODE}]${ColorOff}"
fi

printf "[%b] %b%b%b\n" "$LINE1" "$BBlue" "$CWD" "$ColorOff" | bunx cc-safety-net --statusline

# ── Line 2: Context window usage ─────────────────────────────────────────────
printf "[Ctx ] $CTX_BAR $CTX_PCT%%\n"

# ── Line 3: Rate limit usage (only shown when data is available) ──────────────
RATE_LINE=""
if [ -n "$RATE_5H" ]; then
  RATE_5H_INT=$(printf '%.0f' "$RATE_5H")
  RATE_5H_BAR=$(build_bar "$RATE_5H_INT")
  RATE_LINE="5h: $RATE_5H_BAR $RATE_5H_INT%%"
fi
if [ -n "$RATE_7D" ]; then
  RATE_7D_INT=$(printf '%.0f' "$RATE_7D")
  RATE_7D_BAR=$(build_bar "$RATE_7D_INT")
  [ -n "$RATE_LINE" ] && RATE_LINE="${RATE_LINE}  "
  RATE_LINE="${RATE_LINE}7d: $RATE_7D_BAR $RATE_7D_INT%%"
fi
if [ -n "$RATE_LINE" ]; then
  printf "[Rate] $RATE_LINE\n"
fi

# ── Line 4: Cost and duration (only shown when data is available) ─────────────
COST_DUR_LINE=""
if [ -n "$SESSION_COST" ]; then
  COST_STR=$(printf '$%.4f' "$SESSION_COST")
  COST_DUR_LINE="${Green}${COST_STR}${ColorOff}"
fi
if [ -n "$SESSION_DURATION_MS" ]; then
  DURATION_S=$(( SESSION_DURATION_MS / 1000 ))
  DURATION_M=$(( DURATION_S / 60 ))
  DURATION_S_REM=$(( DURATION_S % 60 ))
  if [ "$DURATION_M" -gt 0 ]; then
    DUR_STR="${DURATION_M}m${DURATION_S_REM}s"
  else
    DUR_STR="${DURATION_S_REM}s"
  fi
  [ -n "$COST_DUR_LINE" ] && COST_DUR_LINE="${COST_DUR_LINE}  "
  COST_DUR_LINE="${COST_DUR_LINE}${Cyan}${DUR_STR}${ColorOff}"
fi
if [ -n "$COST_DUR_LINE" ]; then
  printf "[Cost] $COST_DUR_LINE\n"
fi
