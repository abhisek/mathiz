import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  api,
  ApiError,
  type ChildProfile,
  type Quest,
  type QuestQuestion,
  type QuestQuestionInput,
  type QuestStatus,
} from '../../api'
import { useAction } from '../../hooks'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'
import { QUEST_STATUS_LABEL } from './questStatus'

const EMPTY_QUESTION: QuestQuestionInput = {
  text: '',
  answer: '',
  answerType: 'integer',
  format: 'numeric',
  choices: [],
  hint: '',
  explanation: '',
}

// Quest editor as a full page (/dashboard/quests/:id) — the former
// QuestEditorModal with room to breathe: natural page scroll for the
// question list, a sticky-ish header for the actions.
export default function QuestEditor() {
  const { token, children } = useDashboard()
  const kids = children.filter((c) => !c.profile.archived).map((c) => c.profile)
  const { id: questId } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [quest, setQuest] = useState<Quest | null>(null)
  const [questions, setQuestions] = useState<QuestQuestion[]>([])
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // 'new' = the add form, a question id = that question's edit form.
  const [editing, setEditing] = useState<string | null>(null)
  // Non-blocking save warnings from the API's math recompute, keyed by
  // question id — the save succeeded, but double-check for a typo.
  const [warnings, setWarnings] = useState<Record<string, string>>({})
  const [publishError, setPublishError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [editingDetails, setEditingDetails] = useState(false)

  // The page stays mounted when navigating quest → quest (only :id changes),
  // so responses must be dropped once questId moves on — otherwise quest A's
  // data (or its 404) bleeds into quest B's editor.
  const activeQuestId = useRef(questId)

  const load = useCallback(async () => {
    if (!questId) return
    try {
      const res = await api.getQuest(token, questId)
      if (activeQuestId.current !== questId) return
      setQuest(res.quest)
      setQuestions(res.questions ?? [])
      setError(null)
    } catch (err) {
      if (activeQuestId.current !== questId) return
      // Cross-tenant and missing both 404 — same friendly dead end.
      if (err instanceof ApiError && err.status === 404) setNotFound(true)
      else setError(err instanceof Error ? err.message : String(err))
    }
  }, [token, questId])

  // Fresh quest, fresh page: clear everything carried over from the
  // previous :id (loaded quest, sticky notFound, open forms, warnings).
  // Keyed on questId alone — a token refresh must not wipe an open form.
  useEffect(() => {
    activeQuestId.current = questId
    setQuest(null)
    setQuestions([])
    setNotFound(false)
    setError(null)
    setEditing(null)
    setWarnings({})
    setPublishError(null)
    setEditingDetails(false)
  }, [questId])

  useEffect(() => {
    void load()
  }, [load])

  async function saveQuestion(input: QuestQuestionInput, qid: string | null) {
    if (!questId) return
    setBusy(true)
    setError(null)
    try {
      const res = qid
        ? await api.updateQuestQuestion(token, questId, qid, input)
        : await api.addQuestQuestion(token, questId, input)
      setWarnings((w) => {
        const next = { ...w }
        if (res.warning) next[res.question.id] = res.warning
        else delete next[res.question.id]
        return next
      })
      setEditing(null)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function deleteQuestion(qid: string) {
    if (!questId) return
    setError(null)
    try {
      await api.deleteQuestQuestion(token, questId, qid)
      setWarnings((w) => {
        const next = { ...w }
        delete next[qid]
        return next
      })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  async function publish() {
    if (!questId) return
    setBusy(true)
    setPublishError(null)
    try {
      await api.publishQuest(token, questId)
      await load()
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function setStatus(status: string) {
    if (!questId) return
    setBusy(true)
    setPublishError(null)
    try {
      await api.updateQuest(token, questId, { status })
      await load()
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const [removeQuest, deletingQuest] = useAction(async () => {
    if (!quest || !questId) return
    if (!window.confirm(`Delete the quest "${quest.name}"? This cannot be undone.`)) return
    try {
      await api.deleteQuest(token, questId)
      navigate('/dashboard/quests')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  })

  if (!questId || notFound) {
    return (
      <div className="quest-editor">
        <Link className="quest-back" to="/dashboard/quests">
          ← Quests
        </Link>
        <div className="empty">
          <p>That quest doesn't exist (or was deleted).</p>
          <p>
            <Link to="/dashboard/quests">Back to your quests</Link>
          </p>
        </div>
      </div>
    )
  }

  if (!quest) {
    return (
      <div className="quest-editor" aria-hidden={!error}>
        <Link className="quest-back" to="/dashboard/quests">
          ← Quests
        </Link>
        {error ? (
          <p className="form-error">{error}</p>
        ) : (
          <>
            <div className="section-head">
              <Skeleton width="14rem" height="1.4rem" />
              <Skeleton width="5rem" height="1.4rem" />
            </div>
            <Skeleton width="18rem" height="0.85rem" />
          </>
        )}
      </div>
    )
  }

  const targetName = quest.childId
    ? kids.find((k) => k.id === quest.childId)?.name ?? 'Unknown child'
    : 'All children'

  return (
    <div className="quest-editor">
      <div className="quest-editor-head">
        <Link className="quest-back" to="/dashboard/quests">
          ← Quests
        </Link>
        <div className="section-head">
          <h2>
            {quest.emoji || '⭐'} {quest.name}
          </h2>
          <span className={`quest-status quest-status-${quest.status}`}>
            {QUEST_STATUS_LABEL[quest.status] ?? quest.status}
          </span>
        </div>
        <p className="muted">
          For {targetName}
          {quest.skillId ? ` · tagged ${quest.skillId}` : ' · untagged (standalone practice)'}{' '}
          <button className="linklike" onClick={() => setEditingDetails((s) => !s)}>
            {editingDetails ? 'Close details' : 'Edit details'}
          </button>
        </p>
        {publishError && <p className="form-error">{publishError}</p>}
        <div className="quest-editor-actions">
          {quest.status === 'draft' && (
            <button className="btn btn-primary" disabled={busy} onClick={() => void publish()}>
              {busy ? 'Working…' : 'Publish to the map'}
            </button>
          )}
          {quest.status === 'active' && (
            <button
              className="btn btn-secondary"
              disabled={busy}
              onClick={() => void setStatus('draft')}
            >
              Unpublish
            </button>
          )}
          {quest.status === 'active' && (
            <button
              className="btn btn-ghost"
              disabled={busy}
              onClick={() => void setStatus('archived')}
            >
              Archive
            </button>
          )}
          {quest.status === 'archived' && (
            <button
              className="btn btn-secondary"
              disabled={busy}
              onClick={() => void setStatus('draft')}
            >
              Reopen as draft
            </button>
          )}
          <button
            className="btn btn-ghost btn-danger"
            disabled={deletingQuest}
            onClick={() => void removeQuest()}
          >
            {deletingQuest ? 'Deleting…' : 'Delete quest'}
          </button>
        </div>
      </div>

      {editingDetails && (
        <QuestDetailsForm
          key={quest.id}
          token={token}
          quest={quest}
          kids={kids}
          onSaved={async () => {
            setEditingDetails(false)
            await load()
          }}
          onCancel={() => setEditingDetails(false)}
        />
      )}

      {error && <p className="form-error">{error}</p>}

      <h4>Questions</h4>
      {questions.length === 0 && editing !== 'new' && (
        <p className="muted">No questions yet — add one below or let the AI draft some.</p>
      )}
      <ul className="question-list">
        {questions.map((qq, i) =>
          editing === qq.id ? (
            <li key={qq.id} className="question-item question-editing">
              <QuestionForm
                initial={qq}
                busy={busy}
                onSave={(input) => void saveQuestion(input, qq.id)}
                onCancel={() => setEditing(null)}
              />
            </li>
          ) : (
            <li key={qq.id} className="question-item">
              <div className="question-main">
                <span className="question-text">
                  {i + 1}. {qq.text}
                  {qq.generated && (
                    <span className="pin-chip" title="Drafted by AI — review before publishing">
                      AI draft
                    </span>
                  )}
                </span>
                <span className="muted">
                  Answer: <strong>{qq.answer}</strong> · {qq.format}
                  {qq.choices && qq.choices.length > 0
                    ? ` (${qq.choices.join(' / ')})`
                    : ''}
                </span>
                {warnings[qq.id] && <p className="question-warning">⚠️ {warnings[qq.id]}</p>}
              </div>
              <div className="question-actions">
                <button className="btn btn-ghost" onClick={() => setEditing(qq.id)}>
                  Edit
                </button>
                <button
                  className="btn btn-ghost btn-danger"
                  onClick={() => void deleteQuestion(qq.id)}
                >
                  Delete
                </button>
              </div>
            </li>
          ),
        )}
      </ul>

      {editing === 'new' ? (
        <QuestionForm
          initial={null}
          busy={busy}
          onSave={(input) => void saveQuestion(input, null)}
          onCancel={() => setEditing(null)}
        />
      ) : (
        <button className="btn btn-secondary" onClick={() => setEditing('new')}>
          + Add question
        </button>
      )}

      <GenerateBox
        token={token}
        questId={questId}
        status={quest.status}
        onMakeDraft={() => void setStatus('draft')}
        makeDraftBusy={busy}
        onGenerated={load}
      />
    </div>
  )
}

// Rename / re-emoji / retarget / retag the quest (PATCH /api/v1/quests/{id}).
function QuestDetailsForm({
  token,
  quest,
  kids,
  onSaved,
  onCancel,
}: {
  token: string
  quest: Quest
  kids: ChildProfile[]
  onSaved: () => Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState(quest.name)
  const [emoji, setEmoji] = useState(quest.emoji ?? '')
  const [childId, setChildId] = useState(quest.childId)
  const [skillId, setSkillId] = useState(quest.skillId)
  const [error, setError] = useState<string | null>(null)

  const [save, saving] = useAction(async () => {
    try {
      await api.updateQuest(token, quest.id, {
        name: name.trim(),
        emoji,
        childId,
        skillId: skillId.trim(),
      })
      await onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  })

  return (
    <form
      className="question-form quest-details-form"
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <div className="question-form-row">
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Emoji <span className="muted">(optional)</span>
          <input
            value={emoji}
            onChange={(e) => setEmoji(e.target.value)}
            placeholder="⭐"
            maxLength={4}
          />
        </label>
      </div>
      <div className="question-form-row">
        <label>
          For
          <select value={childId} onChange={(e) => setChildId(e.target.value)}>
            <option value="">All children</option>
            {kids.map((k) => (
              <option key={k.id} value={k.id}>
                {k.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Skill tag <span className="muted">(optional)</span>
          <input
            value={skillId}
            onChange={(e) => setSkillId(e.target.value)}
            placeholder="e.g. mult-2digit"
          />
        </label>
      </div>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={onCancel}>
          Cancel
        </button>
        <button className="btn btn-primary" disabled={saving || !name.trim()}>
          {saving ? 'Saving…' : 'Save details'}
        </button>
      </div>
    </form>
  )
}

function QuestionForm({
  initial,
  busy,
  onSave,
  onCancel,
}: {
  initial: QuestQuestion | null
  busy: boolean
  onSave: (input: QuestQuestionInput) => void
  onCancel: () => void
}) {
  const [text, setText] = useState(initial?.text ?? EMPTY_QUESTION.text)
  const [answer, setAnswer] = useState(initial?.answer ?? EMPTY_QUESTION.answer)
  const [format, setFormat] = useState(initial?.format ?? EMPTY_QUESTION.format)
  const [answerType, setAnswerType] = useState(initial?.answerType ?? EMPTY_QUESTION.answerType)
  const [choicesText, setChoicesText] = useState((initial?.choices ?? []).join(', '))
  const [hint, setHint] = useState(initial?.hint ?? EMPTY_QUESTION.hint)
  const [explanation, setExplanation] = useState(initial?.explanation ?? EMPTY_QUESTION.explanation)

  function submit(e: FormEvent) {
    e.preventDefault()
    onSave({
      text: text.trim(),
      answer: answer.trim(),
      answerType,
      format,
      choices:
        format === 'multiple_choice'
          ? choicesText
              .split(',')
              .map((c) => c.trim())
              .filter(Boolean)
          : [],
      hint: hint.trim(),
      explanation: explanation.trim(),
    })
  }

  return (
    <form className="question-form" onSubmit={submit}>
      <label>
        Question
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="e.g. What is the HCF of 12 and 18?"
          required
        />
      </label>
      <div className="question-form-row">
        <label>
          Type
          <select
            value={format}
            onChange={(e) => setFormat(e.target.value as 'numeric' | 'multiple_choice')}
          >
            <option value="numeric">Numeric</option>
            <option value="multiple_choice">Multiple choice</option>
          </select>
        </label>
        <label>
          Answer kind
          <select value={answerType} onChange={(e) => setAnswerType(e.target.value)}>
            <option value="integer">Integer</option>
            <option value="decimal">Decimal</option>
            <option value="fraction">Fraction</option>
            {format === 'multiple_choice' && <option value="text">Text</option>}
          </select>
        </label>
        <label>
          Answer
          <input
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            placeholder="e.g. 6"
            required
          />
        </label>
      </div>
      {format === 'multiple_choice' && (
        <label>
          Choices <span className="muted">(comma-separated, must include the answer)</span>
          <input
            value={choicesText}
            onChange={(e) => setChoicesText(e.target.value)}
            placeholder="e.g. 3, 6, 9, 12"
            required
          />
        </label>
      )}
      <label>
        Hint <span className="muted">(optional)</span>
        <input value={hint} onChange={(e) => setHint(e.target.value)} />
      </label>
      <label>
        Explanation <span className="muted">(optional — shown after a wrong answer)</span>
        <input value={explanation} onChange={(e) => setExplanation(e.target.value)} />
      </label>
      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={onCancel}>
          Cancel
        </button>
        <button className="btn btn-primary" disabled={busy}>
          {busy ? 'Saving…' : initial ? 'Save question' : 'Add question'}
        </button>
      </div>
    </form>
  )
}

// GenerateBox drafts questions with AI. The clientKey is minted ONCE per
// form open and reused on retries, so a retried click can never double-debit
// (the server replays the original result). A fresh key is minted only after
// a successful generation, when the next click really is a new request.
function GenerateBox({
  token,
  questId,
  status,
  onMakeDraft,
  makeDraftBusy,
  onGenerated,
}: {
  token: string
  questId: string
  status: QuestStatus
  onMakeDraft: () => void
  makeDraftBusy: boolean
  onGenerated: () => Promise<void>
}) {
  const [brief, setBrief] = useState('')
  const [count, setCount] = useState(5)
  const [clientKey, setClientKey] = useState(() => crypto.randomUUID())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function generate(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const res = await api.generateQuestQuestions(token, questId, brief.trim(), count, clientKey)
      setClientKey(crypto.randomUUID())
      setBrief('')
      setNotice(
        res.replayed
          ? 'Already generated for this request — showing the saved draft.'
          : `Drafted ${res.questions.length} ${res.questions.length === 1 ? 'question' : 'questions'} — review them above, then publish.`,
      )
      await onGenerated()
    } catch (err) {
      // Keep the same clientKey: retrying this click must not double-debit.
      if (err instanceof ApiError && err.status === 402) {
        setError('Not enough expeditions in the wallet')
      } else {
        setError(err instanceof Error ? err.message : String(err))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="generate-box">
      <h4>✨ Generate with AI</h4>
      {status !== 'draft' ? (
        // Review-before-publish (specs/15-quests.md): AI questions need the
        // parent's eyes before kids see them, so generation is paused while
        // the quest is live. Offer the exit right here, in the UI's own words.
        <div className="generate-paused">
          <p className="muted">
            New AI questions need your review before kids see them, so
            generating is paused while this quest is{' '}
            {status === 'archived' ? 'archived' : 'on the map'}.
          </p>
          <button className="btn btn-secondary" disabled={makeDraftBusy} onClick={onMakeDraft}>
            {makeDraftBusy
              ? 'Working…'
              : status === 'archived'
                ? 'Reopen as draft & generate'
                : 'Take off the map & generate'}
          </button>
          {status === 'active' && (
            <p className="muted generate-paused-note">
              The quest returns to the map when you publish again.
            </p>
          )}
        </div>
      ) : (
        <form onSubmit={generate}>
          <label>
            Brief
            <input
              value={brief}
              onChange={(e) => setBrief(e.target.value)}
              placeholder="e.g. HCF word problems, grade 5 — the count comes from the box below"
              required
            />
          </label>
          <div className="generate-row">
            <label className="generate-count">
              Questions
              <input
                type="number"
                min={1}
                max={20}
                value={count}
                onChange={(e) => setCount(Number(e.target.value))}
              />
            </label>
            <button className="btn btn-primary" disabled={busy || !brief.trim()}>
              {busy ? 'Drafting…' : 'Generate'}
            </button>
          </div>
          <p className="form-hint">
            Drafts are saved for your review — nothing reaches your child until
            you publish.
          </p>
        </form>
      )}
      {error && <p className="form-error">{error}</p>}
      {notice && <p className="form-notice">{notice}</p>}
    </div>
  )
}
