# Terminal demo for the README, rendered with charmbracelet/vhs.
#
#   make demo        # writes demo/demo.gif
#
# The recording runs against a throwaway fixture under /tmp/demo, never
# the machine's real ~/.claude or OpenCode config. Keep it that way:
# this GIF is published, and real session titles are not.

Output demo/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms

Hide
Type "export DEMO=/tmp/demo && rm -rf $DEMO && clear" Enter
Type "mkdir -p $DEMO/cc/projects/-home-alice-projects-api $DEMO/cc/skills $DEMO/oc $DEMO/ocdata" Enter
Type "export CLAUDE_CODE_HOME=$DEMO/cc OPENCODE_CONFIG_HOME=$DEMO/oc OPENCODE_DATA_HOME=$DEMO/ocdata" Enter
Type "export A2MIGRATE_LOG_LEVEL=warn" Enter
Type `printf -- '---\nname: code-review\ndescription: review a diff\n---\nreview it\n' > $DEMO/cc/skills/code-review.md` Enter
Type `printf '# House rules\nprefer table-driven tests\n' > $DEMO/cc/CLAUDE.md` Enter
Type `printf '%s\n%s\n' '{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"add-caching","timestamp":"2026-07-20T10:00:00Z","cwd":"/home/alice/projects/api","message":{"role":"user","content":"add a caching layer"}}' '{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"add-caching","timestamp":"2026-07-20T10:00:01Z","cwd":"/home/alice/projects/api","message":{"role":"assistant","content":[{"type":"text","text":"here is the cache"}],"usage":{"input_tokens":120,"output_tokens":45}}}' > $DEMO/cc/projects/-home-alice-projects-api/add-caching.jsonl` Enter
Type "clear" Enter
Show

Type "# What does Claude Code have?" Sleep 400ms Enter
Type "a2migrate list claude-code sessions" Sleep 300ms Enter
Sleep 2s

Type "" Enter
Type "# Preview the move. Nothing is written." Sleep 400ms Enter
Type "a2migrate migrate claude-code opencode --dry-run" Sleep 300ms Enter
Sleep 2500ms

Type "" Enter
Type "# Do it, with a backup first." Sleep 400ms Enter
Type "a2migrate migrate claude-code opencode --backup --yes" Sleep 300ms Enter
Sleep 3s

Type "" Enter
Type "# Where did it land, and where did it come from?" Sleep 400ms Enter
Type "a2migrate verify opencode" Sleep 300ms Enter
Sleep 2500ms

Type "" Enter
Type "# Swap the arguments to migrate back." Sleep 400ms Enter
Type "a2migrate migrate opencode claude-code sessions --yes" Sleep 300ms Enter
Sleep 3s

Type "" Enter
Type "# Re-running is a no-op." Sleep 400ms Enter
Type "a2migrate migrate claude-code opencode sessions --yes" Sleep 300ms Enter
Sleep 3s

Hide
Type "rm -rf $DEMO" Enter
Show
Sleep 1s
