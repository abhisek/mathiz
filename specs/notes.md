# Notes

These are my notes in building this project almost entirely using Claude Code. I did minimal coding.
I focussed on the domain of the problem, making decision as per my
[taste](https://paulgraham.com/taste.html) and understanding of the domain.

## Goals

- Self-paced progressive learning by demonstration
- Grounded on math concepts backed by K-12 or other relevant system
- Positive reinforcement of effort, learning and mastery

## Non-Goals

- Leader board / competition like experience

## Why AI Native

This is more of an experiment. AI is not required here to overcome any limitation of conventional
software development approach for our current requirements. The purpose of this experiment is to
learn context engineering and figure out if we can engineer the context around an AI in a way where
it produces questions, evaluates answers, tracks progress, makes decisions about how to improve the
skill of a learner. In general, LLMs are used to generate problems while evaluation, assessment and
error diagnosis is done using a conventional (deterministic) approach.

I believe this approach will generate better and personalized questions that is fun, yet effective
for a learner's journey. I also believe the adaptive learning approach around the question system
will make it much faster for a learner to prove mastery and progress. We are not building a
*leader board* here. We don't want another competition. But we want the learner to experience a
self-paced progressive mastery of math concepts. Eventually we want this experience to be available
as a game.

There are known trade-off:

### Explainability

Why did the LLM make a specific decision? Is the decision correct? 

To limit ambiguity, even with the non-deterministic nature of LLMs, we leverage:

- Static skill graph represented as Go struct
- LLM generated question verification using rule based parsing and evaluation
- Hooks for LLM as a judge (reduce error but still non-deterministic)
- Constrain LLM context within minimal and most recent context
- Log every LLM calls in local sqlite database for auditing and debugging
- Rule based (conventional) system for learning evaluation, error diagnosis etc.

### Latency

There is a latency for every LLM call and currently every question is generated on-demand using an
LLM. So the system will not work without an Internet connection and LLM service provider.

## Concepts

### Skill

A skill in a math concept is expressed using a combination of accuracy and speed. Learners
demonstrating skill in a specific concept progresses to the next related concept.

### Skill Graph

Math concepts can be modelled as a [skill dependency graph](#). Using this approach, as the system
designers, we constraint the system and retain the control on math concepts and how they are related
to each other even when an LLM is used to generate a specific problem. 

I have not found any papers (yet) that presents a comprehensive study of LLM's knowledge of math
concepts. Specifically, we are not talking about LLMs ability to solve, which is a well known
limitations. But we betting on LLM's ability to generate an intuitive natural language
representation of a problem which a learner can easily understand and solve.

### Spaced Repetition

This project was born when I saw my daughter unable to divide 700/2. Not as a homework but as a
real-life application when have had to share 700 rupees worth of prize money with one of her
team mate. After thinking, trying to remember the "rules" for division and a few trial and error,
she was able to find the answer. What was clear was, continuous practice is required till certain
concepts (division in this case), within the boundary of human mind becomes fluent. 

I have observed my daughter tends to forget older concepts or at least get rusty to that, when she
is learning and practicing a new concept. When I see this with some awareness, it is relatable and
obvious that it is true not just for her but for me and everyone else. We tend to forget concepts
that we don't practice every day.

A bit of survey led me to discover [a paper](https://arxiv.org/pdf/1712.01856), [a hacker news
thread](https://news.ycombinator.com/item?id=39002138)

### Adaptive Learning

The core idea is:

```
mastery = fn(speed, accuracy)
```

Real life evidence shows that a young learner often forgets earlier concepts that they have not yet
mastered (fluent). Even adults have shown evidence of rusty skills even after having decades of
fluency in a concept. To tackle this, one of the goals of the system is to adapt to the current
state of a learner based on their progress, mastery and periodically check for fluency in previously
mastered skill.

All of this is done using a combination of:

- Mastery evaluator
- Spaced repetition
- Error diagnosis

## Approach

The system was built using [ralph](https://ghuntley.com/ralph/) like approach but not entirely. At a
high level, this is what I did:

- Had a brainstorming conversation with Claude to arrive at the use-cases, TUI experience, limitations
  and clearly defined non-goals
- Work with Claude to break down the system into modules (components). See
[specs/README.md](/specs/README.md)
- Then for each component, work with Claude using prompts like `Interview me to find required
information ..` to write detailed specs for each component. See [specs](../specs/)
- Run Claude to generate code for each spec along with basic test cases
- Manually perform user acceptance testing and feedback reviews to Claude

### Claude Code Harness

- Docker based sandbox using [https://github.com/trailofbits/claude-code-devcontainer](https://github.com/trailofbits/claude-code-devcontainer)
- Claude Code (max)
- Go 1.25.1

## Challenges

### Spec Drift

The most common problem that I anticipated and noticed very quickly. The TUI drifted first because I
want to tweak the experience. Also based on feedback from some of the learners, I had to tweak and
change the design language for the TUI. At this point, I am not sure if its worth keeping the spec
updated.

