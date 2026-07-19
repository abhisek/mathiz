import { useCallback, useEffect, useState, type FormEvent } from 'react'
import type { Session } from '@supabase/supabase-js'
import {
  api,
  ApiError,
  type BillingInfo,
  type ChildProfile,
  type ChildStats,
  type ChildWithSummary,
  type Device,
  type FamilySpace,
  type Invite,
  type Quest,
  type QuestQuestion,
  type QuestQuestionInput,
} from '../api'
import { getSupabase } from '../supa'

interface Props {
  session: Session
}

export default function Dashboard({ session }: Props) {
  const token = session.access_token
  const [family, setFamily] = useState<FamilySpace | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const me = await api.me(token)
      setFamily(me.family)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoaded(true)
    }
  }, [token])

  useEffect(() => {
    void load()
  }, [load])

  async function signOut() {
    const supa = await getSupabase()
    await supa.auth.signOut()
  }

  if (!loaded) return <div className="boot">Loading…</div>

  return (
    <div className="page">
      <header className="topbar">
        <div className="brand brand-small">
          <span className="brand-mark">∑</span>
          <span>Mathiz</span>
        </div>
        <div className="topbar-right">
          <span className="muted">{session.user.email}</span>
          <button className="btn btn-ghost" onClick={signOut}>
            Sign out
          </button>
        </div>
      </header>

      {error && <p className="form-error">{error}</p>}

      {family ? (
        <FamilyView token={token} family={family} />
      ) : (
        <CreateFamily token={token} onCreated={load} />
      )}
    </div>
  )
}

// Skeleton is a shimmering placeholder bar (see .skeleton in index.css).
// Rendered ONLY while a section's first fetch is in flight, shaped like the
// content it stands in for so nothing jumps when the data lands. Empty-state
// copy ("No children yet…") must never show before that first load resolves.
function Skeleton({
  width,
  height = '0.8rem',
  circle = false,
}: {
  width?: string
  height?: string
  circle?: boolean
}) {
  return (
    <div
      aria-hidden="true"
      className={circle ? 'skeleton skeleton-circle' : 'skeleton'}
      style={{ width, height }}
    />
  )
}

function CreateFamily({ token, onCreated }: { token: string; onCreated: () => Promise<void> }) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.createFamily(token, name)
      await onCreated()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center-card">
      <h2>Create your Family Space</h2>
      <p className="muted">
        A Family Space holds your children's profiles and their learning progress.
      </p>
      <form onSubmit={submit}>
        <label>
          Family name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. The Sharmas"
            required
          />
        </label>
        {error && <p className="form-error">{error}</p>}
        <button className="btn btn-primary" disabled={busy}>
          {busy ? 'Creating…' : 'Create Family Space'}
        </button>
      </form>
    </div>
  )
}

function FamilyView({ token, family }: { token: string; family: FamilySpace }) {
  const [children, setChildren] = useState<ChildWithSummary[]>([])
  const [invites, setInvites] = useState<Invite[]>([])
  const [showAddChild, setShowAddChild] = useState(false)
  const [openChild, setOpenChild] = useState<string | null>(null)
  // True until the FIRST refresh resolves — drives the skeletons. Later
  // refreshes (after mutations) never flip it back.
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [kids, invs] = await Promise.all([
        api.listChildren(token, family.id),
        api.listInvites(token, family.id),
      ])
      setChildren(kids.children ?? [])
      setInvites(invs.invites ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [token, family.id])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const [inviteDays, setInviteDays] = useState(7)

  const [minting, setMinting] = useState(false)

  async function mintInvite() {
    if (minting) return
    setMinting(true)
    try {
      await api.createInvite(token, family.id, inviteDays * 24)
      // Only the invite list changed — don't refetch every child's stats.
      const invs = await api.listInvites(token, family.id)
      setInvites(invs.invites ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setMinting(false)
    }
  }

  async function revokeInvite(id: string) {
    try {
      await api.revokeInvite(token, id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="family">
      <div className="family-header">
        <h2>{family.name}</h2>
        <button className="btn btn-primary" onClick={() => setShowAddChild(true)}>
          + Add child
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}

      {(() => {
        const active = children.filter((c) => !c.profile.archived)
        const noPin = active.filter((c) => !c.profile.hasPin)
        if (active.length < 2 || noPin.length === 0) return null
        return (
          <div className="tip-card">
            🔒 With more than one explorer aboard, a PIN on each profile keeps
            siblings from mixing up each other's maps. Missing a PIN:{' '}
            <strong>{noPin.map((c) => c.profile.name).join(', ')}</strong> —
            open their card below to set one.
          </div>
        )
      })()}

      <section className="cards">
        {loading &&
          [0, 1].map((i) => (
            <div key={i} className="child-card" aria-hidden="true">
              <div className="child-card-head" style={{ cursor: 'default' }}>
                <Skeleton circle width="2.6rem" height="2.6rem" />
                <div className="child-meta" style={{ gap: '0.4rem' }}>
                  <Skeleton width="7rem" height="0.95rem" />
                  <Skeleton width="4.5rem" height="0.7rem" />
                </div>
                <Skeleton width="5.5rem" height="0.9rem" />
                <Skeleton width="100%" height="8px" />
              </div>
            </div>
          ))}
        {!loading && children.length === 0 && (
          <div className="empty">
            <p>No children yet. Add your first learner to get started!</p>
          </div>
        )}
        {children.map((c) => (
          <ChildCard
            key={c.profile.id}
            token={token}
            child={c}
            open={openChild === c.profile.id}
            onToggle={() =>
              setOpenChild(openChild === c.profile.id ? null : c.profile.id)
            }
            onChanged={refresh}
          />
        ))}
      </section>

      <QuestsSection
        token={token}
        familyId={family.id}
        kids={children.filter((c) => !c.profile.archived).map((c) => c.profile)}
      />

      <BillingCard token={token} familyId={family.id} />

      <section className="invites">
        <div className="section-head">
          <h3>Join codes</h3>
          <div className="invite-mint">
            <label className="invite-ttl">
              Expires in{' '}
              <select
                value={inviteDays}
                onChange={(e) => setInviteDays(Number(e.target.value))}
              >
                <option value={7}>7 days</option>
                <option value={30}>30 days</option>
                <option value={90}>90 days</option>
              </select>
            </label>
            <button className="btn btn-secondary" onClick={mintInvite} disabled={minting}>
              {minting ? 'Minting…' : 'New join code'}
            </button>
          </div>
        </div>
        <p className="muted">
          Share a code with your child. They open <code>{window.location.origin}/join</code>,
          type the code, pick their name, and start learning — no email needed.
        </p>
        {loading ? (
          <ul className="invite-list" aria-hidden="true">
            {[0, 1].map((i) => (
              <li key={i}>
                <Skeleton width="6.5rem" height="1.8rem" />
                <Skeleton width="9rem" height="0.8rem" />
              </li>
            ))}
          </ul>
        ) : invites.length === 0 ? (
          <p className="muted">No active codes.</p>
        ) : (
          <ul className="invite-list">
            {invites.map((inv) => (
              <li key={inv.id}>
                <code className="join-code">{inv.code}</code>
                <span className="muted">
                  expires {new Date(inv.expiresAt).toLocaleDateString()}
                </span>
                <button className="btn btn-ghost btn-danger" onClick={() => revokeInvite(inv.id)}>
                  Revoke
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {showAddChild && (
        <AddChildModal
          token={token}
          familyId={family.id}
          onClose={() => setShowAddChild(false)}
          onAdded={async () => {
            setShowAddChild(false)
            await refresh()
          }}
        />
      )}
    </div>
  )
}

// ---- Parent quests (specs/15-quests.md) ----
// One-off practice sets the parent authors (or AI-drafts) that show up on
// the kid's treasure map. Hidden entirely when the server runs without
// quests (the list endpoint 404s).

const QUEST_STATUS_LABEL: Record<string, string> = {
  draft: 'Draft',
  active: 'On the map',
  archived: 'Archived',
}

function QuestsSection({
  token,
  familyId,
  kids,
}: {
  token: string
  familyId: string
  kids: ChildProfile[]
}) {
  const [quests, setQuests] = useState<Quest[]>([])
  const [hidden, setHidden] = useState(false)
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const res = await api.listQuests(token, familyId)
      setQuests(res.quests ?? [])
      setError(null)
    } catch (err) {
      // Quests disabled server-side → hide the whole section.
      if (err instanceof ApiError && err.status === 404) setHidden(true)
      else setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [token, familyId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  if (hidden) return null

  // First load: an anonymous skeleton (no "Quests" heading — the section may
  // turn out to be disabled server-side, and a title that vanishes is worse
  // than a grey bar that does).
  if (loading) {
    return (
      <section className="quests" aria-hidden="true">
        <div className="section-head">
          <Skeleton width="6rem" height="1.2rem" />
        </div>
        <ul className="quest-list">
          <li>
            <div className="quest-row" style={{ cursor: 'default' }}>
              <Skeleton circle width="1.5rem" height="1.5rem" />
              <div className="quest-row-name" style={{ gap: '0.35rem' }}>
                <Skeleton width="10rem" height="0.95rem" />
                <Skeleton width="14rem" height="0.7rem" />
              </div>
              <Skeleton width="4.5rem" height="1.4rem" />
            </div>
          </li>
        </ul>
      </section>
    )
  }

  function targetName(childId: string) {
    if (!childId) return 'All children'
    return kids.find((k) => k.id === childId)?.name ?? 'Unknown child'
  }

  async function remove(q: Quest) {
    if (!window.confirm(`Delete the quest "${q.name}"? This cannot be undone.`)) return
    try {
      await api.deleteQuest(token, q.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="quests">
      <div className="section-head">
        <h3>Quests</h3>
        <button className="btn btn-secondary" onClick={() => setShowCreate(true)}>
          + New quest
        </button>
      </div>
      <p className="muted">
        A quest is a one-off set of questions you write (or let the AI draft) —
        it appears as a special spot on your child's treasure map.
      </p>
      {error && <p className="form-error">{error}</p>}
      {quests.length === 0 ? (
        <p className="muted">No quests yet.</p>
      ) : (
        <ul className="quest-list">
          {quests.map((q) => (
            <li key={q.id}>
              <button className="quest-row" onClick={() => setEditingId(q.id)}>
                <span className="quest-row-emoji">{q.emoji || '⭐'}</span>
                <span className="quest-row-name">
                  <strong>{q.name}</strong>
                  <span className="muted">
                    {targetName(q.childId)} · {q.questionCount}{' '}
                    {q.questionCount === 1 ? 'question' : 'questions'}
                    {q.skillId ? ` · ${q.skillId}` : ''}
                  </span>
                </span>
                <span className={`quest-status quest-status-${q.status}`}>
                  {QUEST_STATUS_LABEL[q.status] ?? q.status}
                </span>
              </button>
              <button className="btn btn-ghost btn-danger" onClick={() => void remove(q)}>
                Delete
              </button>
            </li>
          ))}
        </ul>
      )}

      {showCreate && (
        <CreateQuestModal
          token={token}
          familyId={familyId}
          kids={kids}
          onClose={() => setShowCreate(false)}
          onCreated={async (q) => {
            setShowCreate(false)
            await refresh()
            setEditingId(q.id)
          }}
        />
      )}

      {editingId && (
        <QuestEditorModal
          token={token}
          questId={editingId}
          kids={kids}
          onClose={async () => {
            setEditingId(null)
            await refresh()
          }}
        />
      )}
    </section>
  )
}

function CreateQuestModal({
  token,
  familyId,
  kids,
  onClose,
  onCreated,
}: {
  token: string
  familyId: string
  kids: ChildProfile[]
  onClose: () => void
  onCreated: (q: Quest) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [emoji, setEmoji] = useState('')
  const [childId, setChildId] = useState('')
  const [skillId, setSkillId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const q = await api.createQuest(token, familyId, {
        name,
        emoji,
        skillId: skillId.trim(),
        childId,
      })
      await onCreated(q)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>New quest</h3>
        <form onSubmit={submit}>
          <label>
            Name
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. HCF & LCM revision"
              required
            />
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
            <span className="form-hint">
              A skill id from the treasure map — tagged quests also push the
              main map forward. Leave blank for standalone practice.
            </span>
          </label>
          {error && <p className="form-error">{error}</p>}
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button className="btn btn-primary" disabled={busy}>
              {busy ? 'Creating…' : 'Create quest'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

const EMPTY_QUESTION: QuestQuestionInput = {
  text: '',
  answer: '',
  answerType: 'integer',
  format: 'numeric',
  choices: [],
  hint: '',
  explanation: '',
}

function QuestEditorModal({
  token,
  questId,
  kids,
  onClose,
}: {
  token: string
  questId: string
  kids: ChildProfile[]
  onClose: () => Promise<void>
}) {
  const [quest, setQuest] = useState<Quest | null>(null)
  const [questions, setQuestions] = useState<QuestQuestion[]>([])
  const [error, setError] = useState<string | null>(null)
  // 'new' = the add form, a question id = that question's edit form.
  const [editing, setEditing] = useState<string | null>(null)
  // Non-blocking save warnings from the API's math recompute, keyed by
  // question id — the save succeeded, but double-check for a typo.
  const [warnings, setWarnings] = useState<Record<string, string>>({})
  const [publishError, setPublishError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await api.getQuest(token, questId)
      setQuest(res.quest)
      setQuestions(res.questions ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [token, questId])

  useEffect(() => {
    void load()
  }, [load])

  async function saveQuestion(input: QuestQuestionInput, qid: string | null) {
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

  const targetName = quest?.childId
    ? kids.find((k) => k.id === quest.childId)?.name ?? 'Unknown child'
    : 'All children'

  return (
    <div className="modal-backdrop" onClick={() => void onClose()}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        {!quest ? (
          <p className="muted">Loading quest…</p>
        ) : (
          <>
            <div className="section-head">
              <h3>
                {quest.emoji || '⭐'} {quest.name}
              </h3>
              <span className={`quest-status quest-status-${quest.status}`}>
                {QUEST_STATUS_LABEL[quest.status] ?? quest.status}
              </span>
            </div>
            <p className="muted">
              For {targetName}
              {quest.skillId ? ` · tagged ${quest.skillId}` : ' · untagged (standalone practice)'}
            </p>
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
              enabled={quest.status === 'draft'}
              onGenerated={load}
            />

            {publishError && <p className="form-error">{publishError}</p>}
            <div className="modal-actions">
              <button className="btn btn-ghost" onClick={() => void onClose()}>
                Close
              </button>
              {quest.status === 'draft' && (
                <button className="btn btn-primary" disabled={busy} onClick={() => void publish()}>
                  {busy ? 'Working…' : 'Publish to the map'}
                </button>
              )}
              {quest.status === 'active' && (
                <button
                  className="btn btn-secondary"
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
            </div>
          </>
        )}
      </div>
    </div>
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
  enabled,
  onGenerated,
}: {
  token: string
  questId: string
  enabled: boolean
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
      {!enabled ? (
        <p className="muted">
          Generation drafts into draft quests only — set the quest back to
          draft first.
        </p>
      ) : (
        <form onSubmit={generate}>
          <label>
            Brief
            <input
              value={brief}
              onChange={(e) => setBrief(e.target.value)}
              placeholder="e.g. 10 HCF word problems, grade 5"
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

// BillingCard shows the expedition wallet. Hidden entirely when the server
// runs without billing (self-hosted free mode → the endpoint 404s; the
// plans guard also covers any malformed response so a billing hiccup can
// never blank the whole dashboard).
function BillingCard({ token, familyId }: { token: string; familyId: string }) {
  const [info, setInfo] = useState<BillingInfo | null>(null)
  const [hidden, setHidden] = useState(false)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .billing(token, familyId)
      .then(setInfo)
      .catch(() => setHidden(true))
      .finally(() => setLoading(false))
  }, [token, familyId])

  if (hidden) return null

  // First load: anonymous skeleton shaped like the wallet + plan tiles (no
  // heading — self-hosted free mode hides this section entirely).
  if (loading) {
    return (
      <section className="billing" aria-hidden="true">
        <div className="section-head">
          <Skeleton width="9rem" height="1.2rem" />
        </div>
        <div className="wallet">
          <Skeleton width="4.5rem" height="2.1rem" />
          <Skeleton width="11rem" height="0.8rem" />
        </div>
        <div className="plan-grid">
          {[0, 1, 2].map((i) => (
            <div key={i} className="plan-card">
              <Skeleton width="5rem" height="0.95rem" />
              <Skeleton width="3.5rem" height="1.4rem" />
              <Skeleton width="100%" height="0.7rem" />
              <Skeleton width="100%" height="2.2rem" />
            </div>
          ))}
        </div>
      </section>
    )
  }

  if (!info || !Array.isArray(info.plans)) return null

  const currentPlan = info.plans.find((p) => p.id === info.plan)

  async function buy(planId: string) {
    setBusy(planId)
    setError(null)
    try {
      const { url } = await api.billingCheckout(token, familyId, planId)
      window.location.href = url
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(null)
    }
  }

  async function portal() {
    try {
      const { url } = await api.billingPortal(token, familyId)
      window.location.href = url
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="billing">
      <div className="section-head">
        <h3>Expedition wallet</h3>
        {info.status === 'active' && (
          <button className="btn btn-ghost" onClick={() => void portal()}>
            Manage billing
          </button>
        )}
      </div>
      <div className="wallet">
        <span className="wallet-balance">⛵ {info.balance}</span>
        <span className="muted">
          expeditions left
          {info.status === 'active' && currentPlan
            ? ` · ${currentPlan.name} plan${info.periodEnd ? `, renews ${new Date(info.periodEnd).toLocaleDateString()}` : ''}`
            : ' · no plan yet'}
        </span>
      </div>
      {info.balance <= 10 && (
        <p className="wallet-low">
          Running low — pick a plan so the crew can keep exploring.
        </p>
      )}
      {error && <p className="form-error">{error}</p>}
      <div className="plan-grid">
        {info.plans.map((p) => (
          <div key={p.id} className={`plan-card${p.id === info.plan ? ' plan-current' : ''}`}>
            <strong>{p.name}</strong>
            <span className="plan-price">
              ${(p.priceUsdCents / 100).toFixed(0)}
              {p.monthlyCredits ? '/mo' : ''}
            </span>
            <span className="muted plan-blurb">{p.blurb}</span>
            <button
              className={p.monthlyCredits ? 'btn btn-primary' : 'btn btn-secondary'}
              disabled={busy !== null || p.id === info.plan}
              onClick={() => void buy(p.id)}
            >
              {p.id === info.plan ? 'Current plan' : busy === p.id ? 'Opening…' : p.monthlyCredits ? 'Subscribe' : 'Buy pack'}
            </button>
          </div>
        ))}
      </div>
    </section>
  )
}

function AddChildModal({
  token,
  familyId,
  onClose,
  onAdded,
}: {
  token: string
  familyId: string
  onClose: () => void
  onAdded: () => Promise<void>
}) {
  const [name, setName] = useState('')
  const [grade, setGrade] = useState(3)
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.addChild(token, familyId, name, grade, pin)
      await onAdded()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>Add a child</h3>
        <form onSubmit={submit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Grade
            <select value={grade} onChange={(e) => setGrade(Number(e.target.value))}>
              {[2, 3, 4, 5].map((g) => (
                <option key={g} value={g}>
                  Grade {g}
                </option>
              ))}
            </select>
          </label>
          <label>
            PIN <span className="muted">(optional, 4–6 digits — stops siblings swapping profiles)</span>
            <input
              value={pin}
              onChange={(e) => setPin(e.target.value)}
              inputMode="numeric"
              pattern="\d{4,6}"
              placeholder="e.g. 4321"
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button className="btn btn-primary" disabled={busy}>
              {busy ? 'Adding…' : 'Add child'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ChildCard({
  token,
  child,
  open,
  onToggle,
  onChanged,
}: {
  token: string
  child: ChildWithSummary
  open: boolean
  onToggle: () => void
  onChanged: () => Promise<void>
}) {
  const { profile, summary } = child
  const [stats, setStats] = useState<ChildStats | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    void api.childStats(token, profile.id).then(setStats).catch(() => {})
    void api
      .listDevices(token, profile.id)
      .then((d) => setDevices(d.devices ?? []))
      .catch(() => {})
  }, [open, token, profile.id])

  async function revokeDevice(id: string) {
    try {
      await api.revokeDevice(token, id)
      const d = await api.listDevices(token, profile.id)
      setDevices(d.devices ?? [])
      setActionError(null)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  async function archive() {
    if (!window.confirm(`Archive ${profile.name}? Their devices will be signed out.`)) return
    try {
      await api.updateChild(token, profile.id, { archived: true })
      await onChanged()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  const pct =
    summary.totalSkills > 0 ? Math.round((summary.masteredSkills / summary.totalSkills) * 100) : 0

  return (
    <div className={`child-card${open ? ' open' : ''}`}>
      <button className="child-card-head" onClick={onToggle}>
        <div className="avatar">{profile.name.charAt(0).toUpperCase()}</div>
        <div className="child-meta">
          <strong>{profile.name}</strong>
          <span className="muted">
            Grade {profile.grade}
            {!profile.hasPin && <span className="pin-chip">no PIN</span>}
          </span>
        </div>
        <div className="child-numbers">
          <span title="Skills mastered">🏆 {summary.masteredSkills}</span>
          <span title="Gems earned">💎 {summary.gems}</span>
        </div>
        <div className="progress">
          <div className="progress-bar" style={{ width: `${pct}%` }} />
        </div>
      </button>

      {open && (
        <div className="child-detail">
          {actionError && <p className="form-error">{actionError}</p>}
          {summary.lastSessionAt ? (
            <p className="muted">
              Last practice: {new Date(summary.lastSessionAt).toLocaleString()}
            </p>
          ) : (
            <p className="muted">No practice sessions yet.</p>
          )}

          {stats?.mastery.strands && (
            <div className="islands-progress">
              <h4>Island progress</h4>
              {stats.mastery.strands.map((s) => (
                <div key={s.id} className="island-row">
                  <span className="island-label">🏝️ {s.name}</span>
                  <div className="progress island-bar">
                    <div
                      className="progress-bar"
                      style={{ width: `${s.total > 0 ? (s.mastered / s.total) * 100 : 0}%` }}
                    />
                  </div>
                  <span className="muted">
                    {s.mastered}/{s.total}
                  </span>
                </div>
              ))}
            </div>
          )}

          {stats?.learnerProfile && (
            <div className="learner-profile">
              <h4>What the AI tutor has learned about {profile.name}</h4>
              <p>{stats.learnerProfile.summary}</p>
              {stats.learnerProfile.strengths?.length > 0 && (
                <p>
                  <strong>Strengths:</strong> {stats.learnerProfile.strengths.join(' · ')}
                </p>
              )}
              {stats.learnerProfile.weaknesses?.length > 0 && (
                <p>
                  <strong>Working on:</strong> {stats.learnerProfile.weaknesses.join(' · ')}
                </p>
              )}
            </div>
          )}

          {stats && stats.recentSessions.length > 0 && (
            <>
              <h4>Recent sessions</h4>
              <table className="sessions">
                <thead>
                  <tr>
                    <th>When</th>
                    <th>Questions</th>
                    <th>Correct</th>
                    <th>Gems</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.recentSessions.map((s, i) => (
                    <tr key={i}>
                      <td>{new Date(s.at).toLocaleString()}</td>
                      <td>{s.questions}</td>
                      <td>{s.correct}</td>
                      <td>{s.gems}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          <PinControl token={token} profile={profile} onChanged={onChanged} />

          <h4>Devices</h4>
          {devices.length === 0 ? (
            <p className="muted">No devices connected yet — share a join code.</p>
          ) : (
            <ul className="device-list">
              {devices.map((d) => (
                <li key={d.id}>
                  <span>{d.label || 'Unnamed device'}</span>
                  <span className="muted">
                    {d.lastUsedAt
                      ? `last used ${new Date(d.lastUsedAt).toLocaleDateString()}`
                      : 'never used'}
                  </span>
                  <button className="btn btn-ghost btn-danger" onClick={() => revokeDevice(d.id)}>
                    Sign out
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div className="danger-zone">
            <button className="btn btn-ghost btn-danger" onClick={archive}>
              Archive {profile.name}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// Set / change a child's PIN after creation. Encouraged (nudge + chip),
// never forced.
function PinControl({
  token,
  profile,
  onChanged,
}: {
  token: string
  profile: ChildProfile
  onChanged: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function save(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.updateChild(token, profile.id, { pin })
      await onChanged()
      setEditing(false)
      setPin('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="pin-control">
      <h4>Profile PIN</h4>
      {!editing ? (
        <p className="muted pin-status">
          {profile.hasPin ? (
            <>PIN is set — only {profile.name} can open this profile. </>
          ) : (
            <>
              No PIN — anyone with the join code can open {profile.name}'s
              profile.{' '}
            </>
          )}
          <button className="linklike" onClick={() => setEditing(true)}>
            {profile.hasPin ? 'Change PIN' : 'Set a PIN'}
          </button>
        </p>
      ) : (
        <form className="pin-form" onSubmit={save}>
          <input
            value={pin}
            onChange={(e) => setPin(e.target.value)}
            placeholder="4–6 digits"
            inputMode="numeric"
            pattern="[0-9]{4,6}"
            minLength={4}
            maxLength={6}
            required
            autoFocus
          />
          <button className="btn btn-primary" disabled={busy}>
            {busy ? 'Saving…' : 'Save PIN'}
          </button>
          <button
            className="btn btn-ghost"
            type="button"
            onClick={() => {
              setEditing(false)
              setPin('')
              setError(null)
            }}
          >
            Cancel
          </button>
          {error && <p className="form-error">{error}</p>}
        </form>
      )}
    </div>
  )
}
