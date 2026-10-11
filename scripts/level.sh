#!/usr/bin/env bash
# usage: level.sh <Name> <target_level> [max_minutes]
# "Continue leveling": follows the Zygor route in order (auto.run), grinds one level at a time when the
# next quest needs a higher level (auto.grind), skips the quests auto cannot do. The session must be
# started (start.sh). Short lines on stdout, details in logs/level_<Name>.log. Exit 0 = level reached
# or route finished, 1 = stopped on an incident (read the last lines), 2 = time limit.
# Long: run it in the background and read the log, or rerun it (it resumes from the character's state).
set -u
cd "$(dirname "$0")/../module" || exit 1
go build -o bin/guidecheck ./cmd/guidecheck || exit 1
exec ./bin/guidecheck -play -char "$1" -until "$2" -max_min "${3:-180}"
