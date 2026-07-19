import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError, type ChildProfile, type Quest } from '../../api'
import { track } from '../../analytics'
import { useAction } from '../../hooks'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'
import { QUEST_STATUS_LABEL } from './questStatus'

// ---- Parent quests (specs/15-quests.md) ----
// One-off practice sets the parent authors (or AI-drafts) that show up on
// the kid's treasure map. When the server runs without quests (the list
// endpoint 404s) the page shows an explicit "not enabled on this server"
// note — the nav item stays, since the layout can't know without probing.

export default function Quests() {
  const { token, family, children } = useDashboard()
  const familyId = family.id
  const kids = children.filter((c) => !c.profile.archived).map((c) => c.profile)
  const navigate = useNavigate()

  const [quests, setQuests] = useState<Quest[]>([])
  const [hidden, setHidden] = useState(false)
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [showArchived, setShowArchived] = useState(false)
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

  const [deletingQuestId, setDeletingQuestId] = useState<string | null>(null)

  // Full refresh on delete is deliberate: quests touch the map and counts.
  const [remove, deleting] = useAction(async (q: Quest) => {
    if (!window.confirm(`Delete the quest "${q.name}"? This cannot be undone.`)) return
    setDeletingQuestId(q.id)
    try {
      await api.deleteQuest(token, q.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setDeletingQuestId(null)
    }
  })

  if (hidden) {
    return (
      <section className="quests">
        <p className="muted">Quests are not enabled on this server.</p>
      </section>
    )
  }

  // First load: an anonymous skeleton (the section may turn out to be
  // disabled server-side, and a title that vanishes is worse than a grey
  // bar that does).
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

  const current = quests.filter((q) => q.status !== 'archived')
  const archived = quests.filter((q) => q.status === 'archived')

  const renderRow = (q: Quest) => (
    <li key={q.id}>
      <button className="quest-row" onClick={() => navigate(`/dashboard/quests/${q.id}`)}>
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
      <button
        className="btn btn-ghost btn-danger"
        disabled={deleting}
        onClick={() => void remove(q)}
      >
        {deleting && deletingQuestId === q.id ? 'Deleting…' : 'Delete'}
      </button>
    </li>
  )

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
      {current.length === 0 && archived.length === 0 ? (
        <p className="muted">No quests yet.</p>
      ) : (
        <>
          {current.length === 0 ? (
            <p className="muted">No active or draft quests.</p>
          ) : (
            <ul className="quest-list">{current.map(renderRow)}</ul>
          )}
          {archived.length > 0 && (
            <div className="quests-archived">
              <button className="linklike" onClick={() => setShowArchived((s) => !s)}>
                {showArchived
                  ? 'Hide archived'
                  : `Show archived (${archived.length})`}
              </button>
              {showArchived && <ul className="quest-list">{archived.map(renderRow)}</ul>}
            </div>
          )}
        </>
      )}

      {showCreate && (
        <CreateQuestModal
          token={token}
          familyId={familyId}
          kids={kids}
          onClose={() => setShowCreate(false)}
          onCreated={async (q) => {
            // Straight into the editor page — the list refetches on return.
            navigate(`/dashboard/quests/${q.id}`)
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
      track.questCreated()
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
