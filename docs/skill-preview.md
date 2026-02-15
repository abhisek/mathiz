# Skill Preview

Test LLM-generated questions for any skill without touching the database. Useful for evaluating question quality, testing new skills, and iterating on prompts.

## Find a skill ID

```sh
mathiz skill list                                  # all skills
mathiz skill list --grade 4                        # grade 4 only
mathiz skill list --strand fractions               # one strand
```

Output includes the skill ID, name, grade, strand, and Common Core standard ID (CCSS).

## Preview questions

```sh
mathiz preview --skill add-3digit                  # 5 Learn-tier questions (default)
mathiz preview --skill add-3digit --tier prove     # Prove-tier questions
mathiz preview --skill add-3digit --count 3        # only 3 questions
```

You can also use a Common Core ID instead of the skill ID:

```sh
mathiz preview --skill 3.NBT.A.2
```

If the CCSS maps to multiple skills, the error message lists the matching IDs so you can pick one.

## What happens

1. The LLM generates a question for the skill and tier.
2. You type your answer at the prompt.
3. Feedback is shown (correct/wrong + explanation).
4. After all questions, a short accuracy summary is printed.

No database is used — no events, snapshots, or mastery state are recorded.

## Available strands

| Flag value | Strand |
|------------|--------|
| `number-and-place-value` | Number & Place Value |
| `addition-and-subtraction` | Addition & Subtraction |
| `multiplication-and-division` | Multiplication & Division |
| `fractions` | Fractions |
| `measurement` | Measurement |
