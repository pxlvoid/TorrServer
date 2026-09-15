// homelab: "Watching now" — a card of its own above the torrent list, under the disk cache summary (rendered by
// CacheSummary: the torrent list is a grid, both span its whole width). Only while someone watches, whatever the
// disk cache. Per client: the player / device, what (title and episode), where the player is, how fast it takes the
// data, since when and how much was sent (streams.js). A click opens the details (StreamsDialog).
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'

import './i18n'
import { parseTitle } from './parseTitle'
import HomelabStreamsDialog from './StreamsDialog'
import { episodeOf, useHomelabStreams } from './streams'
import { WatchCard } from './style'

const clock = unix => new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

export default function HomelabStreamsSummary() {
  const { t } = useTranslation()
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const clients = (useHomelabStreams() || []).filter(c => !c.ended)
  const dialog = isDialogOpen && <HomelabStreamsDialog handleClose={() => setIsDialogOpen(false)} />
  if (!clients.length) return dialog || null
  const total = clients.reduce((sum, c) => sum + c.speed, 0)

  return (
    <>
      <WatchCard onClick={() => setIsDialogOpen(true)} title={t('Homelab.StreamsOpen')}>
        <div className='watch-head'>
          <span>▶ {t('Homelab.StreamsWatching', { count: clients.length })}</span>
          {total > 0 && <span className='watch-total'>{humanizeSpeed(total)}</span>}
        </div>
        {clients.map(c => {
          const { title, subtitle } = parseTitle(c.title || c.path)
          return (
            <div
              key={`${c.ip}|${c.ua}|${c.hash}|${c.path}`}
              className={c.active ? 'watch-row' : 'watch-row watch-paused'}
            >
              <span className='watch-device' title={c.ua}>
                {c.device}
              </span>
              <div className='watch-what'>
                <div className='watch-title' title={c.title}>
                  {title}
                  {episodeOf(c.path) && <span className='watch-episode'>{episodeOf(c.path)}</span>}
                  {!episodeOf(c.path) && subtitle && <span className='watch-episode'>{subtitle}</span>}
                </div>
                <div className='watch-bar'>
                  <div style={{ width: `${Math.max(1, c.position * 100)}%` }} />
                </div>
                <div className='watch-meta'>
                  {[
                    `${Math.round(c.position * 100)}%`,
                    t('Homelab.StreamSince', { time: clock(c.since) }),
                    t('Homelab.StreamSent', { size: humanizeSize(c.bytes) || '0' }),
                  ].join(' · ')}
                </div>
              </div>
              <span className='watch-speed'>{c.active ? humanizeSpeed(c.speed) : t('Homelab.StreamPaused')}</span>
            </div>
          )
        })}
      </WatchCard>
      {dialog}
    </>
  )
}
