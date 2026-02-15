# LLM Usage Auditing

Mathiz logs every LLM API call to a local SQLite database. You can inspect usage, costs, and full request/response content.

## List recent LLM events

```sh
mathiz llm list              # last 20 events
mathiz llm list -n 50        # last 50 events
mathiz llm list -p question-gen   # filter by purpose
```

Output columns: ID, timestamp, purpose, model, input/output tokens, latency (ms), success.

### Purpose labels

| Purpose | Description |
|---------|-------------|
| `question-gen` | Math question generation |
| `lesson` | Micro-lesson generation |
| `diagnosis` | Error diagnosis (misconception detection) |
| `session-compress` | Context compression |
| `profile` | Learner profile generation |

## View full request and response

```sh
mathiz llm view <id>
```

Shows metadata (provider, model, tokens, latency) followed by the full request body (system prompt + messages + schema) and the raw JSON response.

## Database location

The SQLite database is at (in priority order):

1. `$MATHIZ_DB` (if set)
2. `$XDG_DATA_HOME/mathiz/mathiz.db`
3. `~/.local/share/mathiz/mathiz.db`

## Raw SQL access

For advanced queries, use SQLite directly:

```sh
# Token usage by purpose
sqlite3 ~/.local/share/mathiz/mathiz.db \
  "SELECT purpose, COUNT(*), SUM(input_tokens), SUM(output_tokens)
   FROM llm_request_events GROUP BY purpose;"

# Failed requests
sqlite3 ~/.local/share/mathiz/mathiz.db \
  "SELECT id, timestamp, purpose, error_message
   FROM llm_request_events WHERE success = 0
   ORDER BY timestamp DESC LIMIT 10;"
```
