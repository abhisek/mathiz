// OpenAI-compatible stub: dispatches on the JSON-schema name in the request.
import http from 'node:http'

const question = {
  question_text: 'What is 12 + 7?',
  format: 'numeric',
  answer: '19',
  answer_type: 'integer',
  choices: [],
  hint: 'Start at 12 and count up 7 more!',
  difficulty: 2,
  explanation: '12 plus 7 makes 19.',
}

const lesson = {
  title: 'Counting On',
  explanation:
    'When you add two numbers, start from the bigger one and count up. It is faster and you make fewer mistakes!',
  worked_example: 'For 12 + 7: start at 12. Count 13, 14, 15, 16, 17, 18, 19. The answer is 19!',
  practice_question: {
    text: 'Your turn: what is 14 + 5?',
    answer: '19',
    answer_type: 'integer',
    explanation: 'Start at 14 and count up five: 15, 16, 17, 18, 19.',
  },
}

function contentFor(body) {
  if (body.includes('micro-lesson')) return lesson
  return question
}

http
  .createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      res.setHeader('Content-Type', 'application/json')
      res.end(
        JSON.stringify({
          id: 'stub',
          object: 'chat.completion',
          choices: [
            {
              index: 0,
              message: { role: 'assistant', content: JSON.stringify(contentFor(body)) },
              finish_reason: 'stop',
            },
          ],
          usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
        }),
      )
    })
  })
  .listen(
    Number(process.env.PORT ?? 9993),
    process.env.HOST ?? '127.0.0.1', // HOST=0.0.0.0 inside containers
    () => console.log(`llm stub on ${process.env.HOST ?? '127.0.0.1'}:${process.env.PORT ?? 9993}`),
  )
