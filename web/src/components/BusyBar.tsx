import { useEffect, useState } from 'react'
import { subscribeApiActivity } from '../api'

// BusyBar: thin indeterminate progress bar pinned to the top of the viewport
// whenever any API request is in flight. It only appears after activity has
// been continuous for ~150ms, so fast calls never flash it.
export default function BusyBar() {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    let timer: number | undefined
    const unsub = subscribeApiActivity((active) => {
      if (active) {
        timer = window.setTimeout(() => setVisible(true), 150)
      } else {
        if (timer !== undefined) {
          window.clearTimeout(timer)
          timer = undefined
        }
        setVisible(false)
      }
    })
    return () => {
      if (timer !== undefined) window.clearTimeout(timer)
      unsub()
    }
  }, [])

  if (!visible) return null
  return <div className="busybar" aria-hidden="true" />
}
