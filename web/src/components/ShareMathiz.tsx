import { useEffect, useState } from 'react'
import { track } from '../analytics'

// "Tell a friend" — shares a recommendation for Mathiz itself. Deliberately
// distinct from the co-parent invite (Family page): that grants access to
// YOUR family's data; this hands a friend the public landing page so they
// start their own. Keep the two visually and verbally separate.
//
// Capability, not device: any browser with a native share sheet (most
// mobiles) gets it in one tap — the message goes out through the parent's
// own channel, which is what makes a recommendation credible. Everywhere
// else a small panel offers copy-paste plus a QR code, because desktop
// sharing is often really desktop-to-phone sharing (another parent scans
// the screen).

function shareUrl(): string {
  return `${window.location.origin}/?ref=share`
}

function shareMessage(): string {
  return `My kid's been doing math adventures on Mathiz — every question is AI-made just for them. It's free right now: ${shareUrl()}`
}

export default function ShareMathiz({ variant }: { variant: 'nav' | 'link' }) {
  const [open, setOpen] = useState(false)

  async function onClick() {
    if (navigator.share) {
      track.shareOpened('native')
      try {
        await navigator.share({ text: shareMessage() })
      } catch {
        // Dismissing the sheet rejects the promise — not an error.
      }
    } else {
      track.shareOpened('panel')
      setOpen(true)
    }
  }

  return (
    <>
      <button
        type="button"
        className={variant === 'nav' ? 'btn btn-ghost' : 'share-footer-link'}
        onClick={() => void onClick()}
      >
        💜 Share<span className="share-btn-full"> Mathiz</span>
      </button>
      {open && <SharePanel onClose={() => setOpen(false)} />}
    </>
  )
}

function SharePanel({ onClose }: { onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const [qr, setQr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    // The QR encoder stays out of the main bundle — the panel is rare.
    import('qrcode-generator')
      .then((mod) => {
        const code = mod.default(0, 'M')
        code.addData(shareUrl())
        code.make()
        if (!cancelled) setQr(code.createDataURL(8, 4))
      })
      .catch(() => {
        // No QR is fine — copy-paste still works.
      })
    return () => {
      cancelled = true
    }
  }, [])

  async function copy() {
    try {
      await navigator.clipboard.writeText(shareMessage())
      track.shareLinkCopied()
      setCopied(true)
    } catch {
      // Clipboard denied — the message is visible to select by hand.
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal share-panel" onClick={(e) => e.stopPropagation()}>
        <h3>Share Mathiz 💜</h3>
        <p className="share-message">{shareMessage()}</p>
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Close
          </button>
          <button type="button" className="btn btn-primary" onClick={() => void copy()}>
            {copied ? 'Copied ✓' : 'Copy message'}
          </button>
        </div>
        {qr && (
          <div className="share-qr">
            <img src={qr} alt="QR code opening the Mathiz home page" />
            <span className="muted">…or let a friend scan this with their phone</span>
          </div>
        )}
      </div>
    </div>
  )
}
