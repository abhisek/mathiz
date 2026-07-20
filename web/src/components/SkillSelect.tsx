import { useEffect, useState } from 'react'
import { cachedCurriculum, type CurriculumIsland } from '../api'

// SkillSelect picks a quest's skill tag by human name instead of a raw
// skill id: one <select> with an optgroup per island and "name (grade N)"
// options, fed by the shared curriculum cache. Value "" = untagged.
//
// Fail-open by design: while the curriculum is loading the select shows a
// single placeholder option, and if the fetch fails we fall back to the
// old free-text skill-id input — quest authoring must never break because
// a catalog endpoint is down (or missing on an older server).
export default function SkillSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (skillId: string) => void
}) {
  const [islands, setIslands] = useState<CurriculumIsland[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    cachedCurriculum()
      .then((c) => {
        if (!cancelled) setIslands(c.islands ?? [])
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (failed) {
    return (
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="e.g. mult-2digit"
      />
    )
  }

  // A tag not in the catalog (older data, hand-typed id) must stay visible
  // and selected rather than silently snapping to "Untagged".
  const known = new Set(islands?.flatMap((i) => i.skills.map((s) => s.id)) ?? [])
  const unknownValue = value !== '' && islands !== null && !known.has(value)

  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} disabled={islands === null}>
      {islands === null ? (
        <option value={value}>Loading skills…</option>
      ) : (
        <>
          <option value="">Untagged — standalone practice</option>
          {unknownValue && <option value={value}>{value}</option>}
          {islands.map((island) => (
            <optgroup key={island.id} label={island.name}>
              {island.skills.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} (grade {s.grade})
                </option>
              ))}
            </optgroup>
          ))}
        </>
      )}
    </select>
  )
}
