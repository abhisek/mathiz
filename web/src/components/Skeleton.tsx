// Skeleton is a shimmering placeholder bar (see .skeleton in index.css).
// Rendered ONLY while a section's first fetch is in flight, shaped like the
// content it stands in for so nothing jumps when the data lands. Empty-state
// copy ("No children yet…") must never show before that first load resolves.
export default function Skeleton({
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
