import { useCallback, useRef, useState } from 'react'

// useAction wraps an async mutation with a busy flag for button feedback
// ("Revoking…", disabled). Re-entrant calls are ignored while the promise is
// in flight — a double-click can never fire the mutation twice.
export function useAction<A extends unknown[]>(
  fn: (...args: A) => Promise<void>,
): [(...args: A) => Promise<void>, boolean] {
  const [busy, setBusy] = useState(false)
  const busyRef = useRef(false)

  const run = useCallback(
    async (...args: A) => {
      if (busyRef.current) return
      busyRef.current = true
      setBusy(true)
      try {
        await fn(...args)
      } finally {
        busyRef.current = false
        setBusy(false)
      }
    },
    [fn],
  )

  return [run, busy]
}
