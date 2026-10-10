#!/bin/sh
# Explicit user-level installation; no service changes or administrator access.
set -eu
source_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
target_bin="$HOME/.local/bin"
skill_target=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir) [ "$#" -ge 2 ] || exit 2; target_bin=$2; shift 2 ;;
    --skill-dir) [ "$#" -ge 2 ] || exit 2; skill_target=$2; shift 2 ;;
    *) echo 'Usage: install_cli.sh [--bin-dir DIRECTORY] [--skill-dir SKILLS_DIRECTORY]' >&2; exit 2 ;;
  esac
done
case "$target_bin" in /*) ;; *) echo 'Bin directory must be absolute' >&2; exit 2 ;; esac
[ -x "$source_root/bin/routepilot" ] || { echo 'Build the CLI with make cli or use the script bundled in RoutePilot.app.' >&2; exit 1; }
[ ! -L "$target_bin" ] || { echo 'Bin directory must not be a symlink' >&2; exit 1; }
mkdir -p "$target_bin"
target="$target_bin/routepilot"
if [ -e "$target" ] || [ -L "$target" ]; then
  [ -L "$target" ] && [ "$(readlink "$target")" = "$source_root/bin/routepilot" ] || { echo 'An unrelated routepilot command already exists; select another bin directory.' >&2; exit 1; }
else
  # A stable App path follows complete App upgrades without copying old binaries.
  ln -s "$source_root/bin/routepilot" "$target"
fi
printf 'CLI: %s\nAdd this directory to PATH if needed: %s\n' "$target" "$target_bin"
if [ -n "$skill_target" ]; then
  case "$skill_target" in /*) ;; *) echo 'Skill directory must be absolute' >&2; exit 2 ;; esac
  skill_source="$source_root/skills/routepilot-control"
  if [ ! -d "$skill_source" ]; then skill_source="$source_root/../Skills/routepilot-control"; fi
  [ -f "$skill_source/SKILL.md" ] || { echo 'Bundled skill not found' >&2; exit 1; }
  [ ! -L "$skill_target" ] || { echo 'Skill parent directory must not be a symlink' >&2; exit 1; }
  mkdir -p "$skill_target"
  [ ! -e "$skill_target/routepilot-control" ] && [ ! -L "$skill_target/routepilot-control" ] || { echo 'Skill already exists; retain it or choose another directory.' >&2; exit 1; }
  cp -R "$skill_source" "$skill_target/routepilot-control"
  printf 'Skill: %s\n' "$skill_target/routepilot-control"
fi
