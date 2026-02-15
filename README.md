# Mathiz

AI-powered math tutor in the terminal. 
Mathiz helps kids (grades 3-5) build math mastery through adaptive practice, spaced repetition, and LLM generated questions.

Built with [Claude Code](https://code.claude.ai) using a spec driven development approach. See
[specs](./specs/) for more details and [notes](/specs/notes.md) for author notes.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/abhisek/mathiz/main/install.sh | sh
```

Or download a binary manually from [GitHub Releases](https://github.com/abhisek/mathiz/releases).

## Setup

Mathiz requires an LLM API key. It automatically discovers keys from standard environment variables (checked in this order):

| Env var | Provider | Default model |
|---------|----------|---------------|
| `GEMINI_API_KEY` | Gemini | gemini-2.0-flash |
| `OPENAI_API_KEY` | OpenAI | gpt-4o-mini |
| `ANTHROPIC_API_KEY` | Anthropic | claude-sonnet-4 |

If you already have one of these set, Mathiz will just work. To override the auto-detected provider or use a custom model:

```sh
export MATHIZ_LLM_PROVIDER=gemini          # force a specific provider
export MATHIZ_GEMINI_MODEL=gemini-2.0-pro  # override default model
```

## Usage

```sh
mathiz          # launch the TUI
mathiz play     # start a practice session directly
mathiz stats    # view learning stats
mathiz llm      # inspect LLM usage
mathiz reset    # reset all progress
```

## Multiple Profiles

All progress is stored in a single SQLite file. Use the `--db` flag to maintain separate profiles for different learners:

```sh
mathiz --db ~/mathiz-alice.db       # Alice's profile
mathiz --db ~/mathiz-bob.db         # Bob's profile
mathiz stats --db ~/mathiz-alice.db # Alice's stats
```

You can also set the `MATHIZ_DB` environment variable instead. The `--db` flag takes priority over the env var.

## Guides

- [LLM Usage Auditing](./docs/llm-usage-auditing.md) — inspect LLM requests, responses, and token usage

## Build from source

Requires Go 1.25+.

```sh
git clone https://github.com/abhisek/mathiz.git
cd mathiz
make
```

## License

MIT
