# Mathiz

AI-powered math tutor for your terminal. Mathiz helps kids (grades 3-5) build math mastery through adaptive practice, spaced repetition, and LLM-generated questions.

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
mathiz reset    # reset all progress
```

## Build from source

Requires Go 1.23+.

```sh
git clone https://github.com/abhisek/mathiz.git
cd mathiz
CGO_ENABLED=0 go build -o mathiz .
```

## License

MIT
