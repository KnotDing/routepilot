#!/bin/sh
# Install only the agent skill from the published RoutePilot source tag.
set -eu
agents=
conflict=overwrite
while [ "$#" -gt 0 ]; do
  case "$1" in
    --agent)
      [ "$#" -ge 2 ] || exit 2
      case "$2" in codex|claude|hermes|cursor|copilot|gemini|opencode|windsurf|cline|roo|kilo|antigravity|openclaw|qwen|amp|pi) ;; *) echo 'Supported agents: codex claude hermes cursor copilot gemini opencode windsurf cline roo kilo antigravity openclaw qwen amp pi' >&2; exit 2 ;; esac
      case " $agents " in *" $2 "*) ;; *) agents="${agents:+$agents }$2" ;; esac
      shift 2 ;;
    --on-conflict) [ "$#" -ge 2 ] || exit 2; conflict=$2; shift 2 ;;
    *) echo 'Usage: install_skill.sh [--agent AGENT]... [--on-conflict overwrite|skip]' >&2; exit 2 ;;
  esac
done
[ -n "$agents" ] || agents=codex
case "$conflict" in overwrite|skip) ;; *) echo 'Conflict policy must be overwrite or skip' >&2; exit 2 ;; esac
command -v curl >/dev/null 2>&1 || { echo 'curl is required' >&2; exit 1; }
install_agent() (
agent=$1
case "$agent" in
  codex) parent="${CODEX_HOME:-$HOME/.codex}/skills" ;;
  claude) parent="$HOME/.claude/skills" ;;
  hermes) parent="${HERMES_HOME:-$HOME/.hermes}/skills" ;;
  cursor) parent="$HOME/.cursor/skills" ;;
  copilot) parent="$HOME/.copilot/skills" ;;
  gemini) parent="$HOME/.gemini/skills" ;;
  opencode) parent="$HOME/.config/opencode/skills" ;;
  windsurf) parent="$HOME/.codeium/windsurf/skills" ;;
  cline) parent="$HOME/.cline/skills" ;;
  roo) parent="$HOME/.roo/skills" ;;
  kilo) parent="$HOME/.kilo/skills" ;;
  antigravity) parent="$HOME/.gemini/config/skills" ;;
  openclaw) parent="$HOME/.openclaw/skills" ;;
  qwen) parent="$HOME/.qwen/skills" ;;
  amp) parent="$HOME/.config/amp/skills" ;;
  pi) parent="${PI_CODING_AGENT_DIR:-$HOME/.pi/agent}/skills" ;;
esac
case "$parent" in /*) ;; *) echo 'Skill directory must be absolute' >&2; exit 2 ;; esac
[ ! -L "$parent" ] || { echo 'Skill parent directory must not be a symlink' >&2; exit 1; }
umask 077
mkdir -p "$parent"
destination="$parent/routepilot-control"
if { [ -e "$destination" ] || [ -L "$destination" ]; } && [ "$conflict" = skip ]; then
  printf 'Skipped; retained existing skill: %s\n' "$destination"
  exit 0
fi
stage=$(mktemp -d "$parent/.routepilot-control.XXXXXX")
backup=
installed=false
cleanup() {
  if [ "$installed" = false ] && [ -n "$backup" ] && [ ! -e "$destination" ] && [ ! -L "$destination" ]; then
    mv "$backup" "$destination" || printf 'Restore manually from: %s\n' "$backup" >&2
  fi
  [ ! -d "$stage" ] || rm -rf "$stage"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
mkdir -p "$stage/agents" "$stage/references"
base='https://gitea.knotding.com/knot/routepilot/raw/tag/v0.6.6/skills/routepilot-control'
for file in SKILL.md agents/openai.yaml references/recipes.md; do
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    "$base/$file" -o "$stage/$file"
  [ -s "$stage/$file" ] || { echo "Empty skill file: $file" >&2; exit 1; }
done
if [ -e "$destination" ] || [ -L "$destination" ]; then
  backup_root="$parent/../routepilot-skill-backups"
  [ ! -L "$backup_root" ] || { echo 'Backup directory must not be a symlink' >&2; exit 1; }
  mkdir -p "$backup_root"
  backup="$backup_root/routepilot-control.$(date -u +%Y%m%dT%H%M%SZ).$$"
  [ ! -e "$backup" ] && [ ! -L "$backup" ] || { echo 'Backup path already exists' >&2; exit 1; }
  mv "$destination" "$backup"
fi
mv "$stage" "$destination"
installed=true
printf 'Installed RoutePilot v0.6.6 skill: %s\n' "$destination"
[ -z "$backup" ] || printf 'Previous contents backed up to: %s\n' "$backup"
)
for agent in $agents; do install_agent "$agent"; done
