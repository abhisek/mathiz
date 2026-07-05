#!/usr/bin/env python3
"""PostToolUse hook: gofmt any .go file the agent just wrote or edited.

Keeps agent-touched files formatted without reformatting the historically
gofmt-unclean parts of the repo (formatting only follows edits).
"""
import json
import subprocess
import sys

try:
    payload = json.load(sys.stdin)
except json.JSONDecodeError:
    sys.exit(0)

path = (payload.get("tool_input") or {}).get("file_path", "")
if path.endswith(".go"):
    subprocess.run(["gofmt", "-w", path], check=False, capture_output=True)
sys.exit(0)
