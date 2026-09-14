// homelab: cache block of the torrent details (hook in DialogTorrentDetailsContent).
// The upstream mini snake piles all cached pieces into one block — fine for a ring cache of CacheSize,
// meaningless for a persistent cache of many gigabytes. In persistent mode it is replaced by a timeline
// of the whole torrent: what is on disk, where the player is and its download window. The detailed
// piece map (button below) stays upstream and is correct in both modes.
import { useContext, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DarkModeContext } from 'components/App'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import { useHomelabCache } from './store'
import { Timeline } from './style'

// colors of the upstream snakes (TorrentCache/snakeSettings.js), but "on disk" is the bright one:
// on a timeline filled reads as brighter, and the player is the red line of the detailed map
const COLORS = {
  dark: { track: '#5c6469', cached: '#dee3e5', window: '#cda184', reader: '#e53935' },
  light: { track: '#dbf2e8', cached: '#4db380', window: '#afa6e3', reader: '#d32f2f' },
}

function DiskTimeline({ cache, totalLength }) {
  const { t } = useTranslation()
  const { isDarkMode } = useContext(DarkModeContext)
  const colors = COLORS[isDarkMode ? 'dark' : 'light']
  const canvasRef = useRef(null)
  const [width, setWidth] = useState(0)

  const { PiecesCount = 0, PiecesLength = 0, Pieces = {}, Readers = [] } = cache
  const pieces = Object.values(Pieces || {})
  const onDisk = pieces.reduce((sum, p) => sum + (p.Size || 0), 0)
  const total = totalLength || PiecesCount * PiecesLength
  const percent = total ? Math.min(100, (onDisk / total) * 100) : 0

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const measure = () => setWidth(canvas.getBoundingClientRect().width)
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !width || !PiecesCount) return
    const dpr = window.devicePixelRatio || 1
    const w = Math.round(width * dpr)
    const h = Math.round(36 * dpr)
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    const x = piece => (piece / PiecesCount) * w

    ctx.fillStyle = colors.track
    ctx.fillRect(0, 0, w, h)

    // cached pieces: a pixel column covers a whole number of pieces and is shaded by the share of them on
    // disk (prefix sums), so a fully cached stretch is solid, without stripes from uneven pieces per pixel
    const prefix = new Float64Array(PiecesCount + 1)
    pieces.forEach(p => {
      if (p.Size && p.Length && p.Id < PiecesCount) prefix[p.Id + 1] = Math.min(1, p.Size / p.Length)
    })
    for (let i = 0; i < PiecesCount; i++) prefix[i + 1] += prefix[i]
    const bins = Math.min(w, PiecesCount)
    ctx.fillStyle = colors.cached
    for (let i = 0; i < bins; i++) {
      const from = Math.floor((i * PiecesCount) / bins)
      const to = Math.max(from + 1, Math.floor(((i + 1) * PiecesCount) / bins))
      const share = (prefix[to] - prefix[from]) / (to - from)
      if (share > 0) {
        ctx.globalAlpha = share
        const x0 = Math.floor((i * w) / bins)
        ctx.fillRect(x0, 0, Math.floor(((i + 1) * w) / bins) - x0, h)
      }
    }
    ctx.globalAlpha = 1

    // player: download window as a strip at the bottom, position as a line
    Readers.forEach(r => {
      ctx.fillStyle = colors.window
      ctx.fillRect(x(r.Start), h - 6 * dpr, Math.max(2 * dpr, x(r.End) - x(r.Start)), 6 * dpr)
      ctx.fillStyle = colors.reader
      ctx.fillRect(x(r.Reader) - dpr, 0, 2 * dpr, h)
    })
  }, [width, PiecesCount, pieces, Readers, colors])

  return (
    <Timeline>
      <div className='timeline-head'>
        <div>
          {t('Homelab.OnDisk')}: <b>{Math.round(percent)}%</b>
        </div>
        <span>{t('Homelab.SizeOf', { size: humanizeSize(onDisk) || '0', total: humanizeSize(total) })}</span>
      </div>
      <canvas ref={canvasRef} />
      <div className='timeline-legend'>
        <span>
          <i style={{ background: colors.cached }} />
          {t('Homelab.LegendCached')}
        </span>
        <span>
          <i style={{ background: colors.reader }} />
          {t('Homelab.LegendPlayer')}
        </span>
        <span>
          <i style={{ background: colors.window }} />
          {t('Homelab.LegendWindow')}
        </span>
      </div>
    </Timeline>
  )
}

export default function HomelabMiniCache({ cache, children }) {
  const data = useHomelabCache()
  if (!data?.usage?.enabled || !cache?.PiecesCount) return children
  const item = data.items?.find(it => it.hash === cache.Hash)
  return <DiskTimeline cache={cache} totalLength={item?.totalLength} />
}
