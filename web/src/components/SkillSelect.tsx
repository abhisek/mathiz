import { useEffect, useMemo, useRef, useState } from 'react'
import { cachedCurriculum, type CurriculumIsland } from '../api'

// SkillSelect picks a quest's skill tag by human name: a searchable
// combobox over the curriculum catalog. Typing filters skills by name,
// search keywords (e.g. "GCF" finds HCF & LCM), island name, and
// "grade N"; an empty query shows the full island-grouped list so the
// catalog stays browsable. Value "" = untagged.
//
// Fail-open by design: while the curriculum is loading the field is
// disabled, and if the fetch fails we fall back to the old free-text
// skill-id input — quest authoring must never break because a catalog
// endpoint is down (or missing on an older server).

interface SkillOption {
  id: string
  label: string
  islandId: string
  haystack: string // lowercase searchable text
}

interface IslandGroup {
  id: string
  name: string
  options: SkillOption[]
}

const UNTAGGED_LABEL = 'Untagged — standalone practice'

export default function SkillSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (skillId: string) => void
}) {
  const [islands, setIslands] = useState<CurriculumIsland[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

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

  const groups = useMemo<IslandGroup[]>(
    () =>
      (islands ?? []).map((island) => ({
        id: island.id,
        name: island.name,
        options: island.skills.map((s) => ({
          id: s.id,
          label: `${s.name} (grade ${s.grade})`,
          islandId: island.id,
          haystack: [s.name, ...(s.keywords ?? []), island.name, `grade ${s.grade}`]
            .join(' ')
            .toLowerCase(),
        })),
      })),
    [islands],
  )

  // Every query token must match somewhere in the haystack (AND search),
  // so "fractions grade 4" narrows rather than widens.
  const tokens = query.toLowerCase().split(/\s+/).filter(Boolean)
  const filtered = groups
    .map((g) => ({
      ...g,
      options: g.options.filter((o) => tokens.every((t) => o.haystack.includes(t))),
    }))
    .filter((g) => g.options.length > 0)
  const showUntagged =
    tokens.length === 0 || tokens.every((t) => UNTAGGED_LABEL.toLowerCase().includes(t))

  // Flat option order for keyboard navigation, mirroring render order.
  const flat: SkillOption[] = [
    ...(showUntagged ? [{ id: '', label: UNTAGGED_LABEL, islandId: '', haystack: '' }] : []),
    ...filtered.flatMap((g) => g.options),
  ]

  // Keep the active row visible as arrow keys move it.
  useEffect(() => {
    listRef.current
      ?.querySelector('[data-active="true"]')
      ?.scrollIntoView({ block: 'nearest' })
  }, [active, open])

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
  const selectedLabel =
    value === ''
      ? UNTAGGED_LABEL
      : (groups.flatMap((g) => g.options).find((o) => o.id === value)?.label ?? value)

  const select = (id: string) => {
    onChange(id)
    setOpen(false)
    setQuery('')
  }

  const openList = () => {
    setOpen(true)
    setQuery('')
    setActive(0)
  }

  return (
    <div className="skill-select" ref={rootRef}>
      <input
        value={open ? query : islands === null ? '' : selectedLabel}
        disabled={islands === null}
        placeholder={islands === null ? 'Loading skills…' : 'Search skills… e.g. HCF, grade 4'}
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
        onFocus={openList}
        onChange={(e) => {
          setQuery(e.target.value)
          setOpen(true)
          setActive(0)
        }}
        onBlur={(e) => {
          // Ignore focus moving inside the widget (option mousedown
          // prevents default, so this only fires on real focus loss).
          if (!rootRef.current?.contains(e.relatedTarget as Node | null)) {
            setOpen(false)
            setQuery('')
          }
        }}
        onKeyDown={(e) => {
          if (!open) {
            if (e.key === 'ArrowDown' || e.key === 'Enter') {
              e.preventDefault()
              openList()
            }
            return
          }
          if (e.key === 'ArrowDown') {
            e.preventDefault()
            setActive((a) => Math.min(a + 1, flat.length - 1))
          } else if (e.key === 'ArrowUp') {
            e.preventDefault()
            setActive((a) => Math.max(a - 1, 0))
          } else if (e.key === 'Enter') {
            // Always swallow Enter while open — never submit the form.
            e.preventDefault()
            if (flat[active]) select(flat[active].id)
          } else if (e.key === 'Escape') {
            e.preventDefault()
            setOpen(false)
            setQuery('')
          }
        }}
      />
      {open && (
        <ul className="skill-select-list" role="listbox" ref={listRef}>
          {flat.length === 0 && <li className="skill-select-empty">No skills match “{query}”</li>}
          {flat.map((o, i) =>
            o.id === '' ? (
              <li
                key="untagged"
                role="option"
                aria-selected={value === ''}
                className="skill-select-option"
                data-active={i === active}
                onMouseDown={(e) => {
                  e.preventDefault()
                  select('')
                }}
                onMouseMove={() => setActive(i)}
              >
                {UNTAGGED_LABEL}
              </li>
            ) : (
              <SkillRow
                key={o.id}
                option={o}
                header={
                  // First option of its island in render order gets the header.
                  filtered.find((g) => g.id === o.islandId)?.options[0]?.id === o.id
                    ? filtered.find((g) => g.id === o.islandId)?.name
                    : undefined
                }
                selected={o.id === value}
                activeRow={i === active}
                onHover={() => setActive(i)}
                onPick={() => select(o.id)}
              />
            ),
          )}
        </ul>
      )}
    </div>
  )
}

function SkillRow({
  option,
  header,
  selected,
  activeRow,
  onHover,
  onPick,
}: {
  option: SkillOption
  header?: string
  selected: boolean
  activeRow: boolean
  onHover: () => void
  onPick: () => void
}) {
  return (
    <>
      {header && (
        <li className="skill-select-group" aria-hidden="true">
          {header}
        </li>
      )}
      <li
        role="option"
        aria-selected={selected}
        className="skill-select-option"
        data-active={activeRow}
        onMouseDown={(e) => {
          e.preventDefault()
          onPick()
        }}
        onMouseMove={onHover}
      >
        {option.label}
      </li>
    </>
  )
}
