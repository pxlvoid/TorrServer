// homelab: "Watching now" — a compact card of its own above the torrent list, under the disk cache summary
// (rendered by CacheSummary: the torrent list is a grid, both span its whole width). Only while someone watches,
// whatever the disk cache. A row per client: poster, title and episode, the player's time, the player and where the
// data comes from, how fast it goes. A click opens the details (StreamsDialog).
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { humanizeSpeed } from 'utils/Utils'

import './i18n'
import { parseTitle } from './parseTitle'
import HomelabStreamsDialog from './StreamsDialog'
import { clientKey, episodeOf, fmtTime, playerTime, sourceOf, useHomelabStreams } from './streams'
import { WatchCard } from './style'

export function useSourceLabel() {
  const { t } = useTranslation()
  return c => {
    const src = sourceOf(c)
    if (src.kind === 'disk') return t('Homelab.StreamSrcDisk')
    if (src.kind === 'partial') return t('Homelab.StreamSrcPartial', { percent: Math.floor(src.share * 100) })
    return ''
  }
}

export default function HomelabStreamsSummary() {
  const { t } = useTranslation()
  const sourceLabel = useSourceLabel()
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const clients = (useHomelabStreams() || []).filter(c => !c.ended)
  const dialog = isDialogOpen && <HomelabStreamsDialog handleClose={() => setIsDialogOpen(false)} />
  if (!clients.length) return dialog || null
  const total = clients.reduce((sum, c) => sum + c.speed, 0)

  return (
    <>
      <WatchCard onClick={() => setIsDialogOpen(true)} title={t('Homelab.StreamsOpen')}>
        {clients.length > 1 && (
          <div className='watch-head'>
            <span>{t('Homelab.StreamsWatching', { count: clients.length })}</span>
            {total > 0 && <span>{humanizeSpeed(total)}</span>}
          </div>
        )}
        {clients.map(c => {
          const { title, subtitle } = parseTitle(c.title || c.path)
          const time = playerTime(c)
          const meta = [
            c.device,
            sourceLabel(c),
            time ? t('Homelab.StreamLeft', { time: fmtTime(time.left) }) : `${Math.round(c.position * 100)}%`,
          ].filter(Boolean)
          return (
            <div key={clientKey(c)} className={c.active ? 'watch-row' : 'watch-row watch-paused'}>
              <div className='watch-poster'>{c.poster ? <img src={c.poster} alt='' loading='lazy' /> : '▶'}</div>
              <div className='watch-main'>
                <div className='watch-title'>
                  <span className={c.active ? 'watch-dot watch-dot-live' : 'watch-dot'} />
                  {title}
                  <span className='watch-sub'>{episodeOf(c.path) || subtitle}</span>
                </div>
                <div className='watch-progress'>
                  <div className='watch-bar'>
                    <div style={{ width: `${Math.max(1, c.position * 100)}%` }} />
                  </div>
                  {time && <span className='watch-time'>{`${fmtTime(time.at)} / ${fmtTime(time.total)}`}</span>}
                </div>
                <div className='watch-meta'>{meta.join(' · ')}</div>
              </div>
              <div className='watch-speed'>{c.active ? humanizeSpeed(c.speed) : t('Homelab.StreamPaused')}</div>
            </div>
          )
        })}
      </WatchCard>
      {dialog}
    </>
  )
}
