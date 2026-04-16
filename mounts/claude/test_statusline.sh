#!/bin/bash
# Test the statusline script with mock JSON input covering all supported fields.
# Usage: bash test_statusline.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo '--- Full input (all fields present) ---'
echo '{
  "model": {
    "display_name": "Claude Sonnet 4.6"
  },
  "workspace": {
    "current_dir": "/home/cledoux/projects/myapp"
  },
  "context_window": {
    "used_percentage": 65
  },
  "rate_limits": {
    "five_hour": {
      "used_percentage": 42
    },
    "seven_day": {
      "used_percentage": 88
    }
  },
  "session_name": "statusline",
  "output_style": {
    "name": "Explanatory"
  },
  "vim": {
    "mode": "INSERT"
  },
  "agent": {
    "name": "my-agent",
    "type": "custom"
  },
  "worktree": {
    "name": "feature-branch",
    "branch": "feat/new-ui"
  },
  "cost_usd": 0.0237,
  "duration_ms": 105000
}' | bash "$SCRIPT_DIR/statusline-command.sh"

echo ''
echo '--- Minimal input (only required fields) ---'
echo '{
  "model": {
    "display_name": "Claude Haiku 4.5"
  },
  "workspace": {
    "current_dir": "/tmp"
  },
  "context_window": {
    "used_percentage": 20
  }
}' | bash "$SCRIPT_DIR/statusline-command.sh"

echo ''
echo '--- High usage (red bars) ---'
echo '{
  "model": {
    "display_name": "Claude Opus 4.6"
  },
  "workspace": {
    "current_dir": "/home/cledoux"
  },
  "context_window": {
    "used_percentage": 95
  },
  "rate_limits": {
    "five_hour": {
      "used_percentage": 83
    },
    "seven_day": {
      "used_percentage": 91
    }
  },
  "cost_usd": 1.2345,
  "duration_ms": 3725000
}' | bash "$SCRIPT_DIR/statusline-command.sh"

echo ''
echo '--- Vim NORMAL mode, no agent/worktree ---'
echo '{
  "model": {
    "display_name": "Claude Sonnet 4.6"
  },
  "workspace": {
    "current_dir": "/home/cledoux/projects"
  },
  "context_window": {
    "used_percentage": 50
  },
  "vim": {
    "mode": "NORMAL"
  },
  "cost_usd": 0.0012,
  "duration_ms": 45000
}' | bash "$SCRIPT_DIR/statusline-command.sh"
