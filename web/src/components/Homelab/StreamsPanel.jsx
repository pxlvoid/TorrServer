// homelab: who is watching this torrent — in the torrent details, under the disk timeline (MiniCache): per client
// the device, the episode, how fast it takes the data (or paused), where the player is, since when (streams.js).
import { useTranslation } from 'react-i18next'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'

import './i18n'
import { episodeOf, fmtTime, playerTime, useTorrentStreams } from './streams'
import { useSourceLabel } from './StreamsSummary'
import { ProgressBar, StreamsBox } from './style'

const clock = unix => new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

export default function HomelabStreamsPanel({ hash, dark }) {
  const { t } = useTranslation()
  const sourceLabel = useSourceLabel()
  const clients = useTorrentStreams(hash)
  if (!clients.length) return null

  return (
    <StreamsBox dark={dark}>
      <div className='streams-title'>
        ▶ {t('Homelab.StreamsTitle', { count: clients.filter(c => !c.ended).length })}
      </div>
      {clients.map(c => {
        const file = (c.path || '').split('/').pop()
        const state = c.ended ? t('Homelab.StreamEnded') : c.active ? humanizeSpeed(c.speed) : t('Homelab.StreamPaused')
        return (
          <div key={`${c.ip}|${c.ua}|${c.path}`} className={c.ended ? 'stream stream-ended' : 'stream'} title={c.ua}>
            <div className='stream-head'>
              <b>{c.device}</b>
              <span className={c.active ? 'stream-speed stream-active' : 'stream-speed'}>{state}</span>
            </div>
            <div className='stream-file'>{episodeOf(c.path) || file}</div>
            <ProgressBar dark={dark} value={c.position * 100} />
            <div className='stream-meta'>
              {[
                playerTime(c)
                  ? `${fmtTime(playerTime(c).at)} / ${fmtTime(playerTime(c).total)}`
                  : `${Math.round(c.position * 100)}%`,
                sourceLabel(c),
                t('Homelab.StreamSince', { time: clock(c.since) }),
                t('Homelab.StreamSent', { size: humanizeSize(c.bytes) || '0' }),
                c.connections > 1 && t('Homelab.StreamConnections', { count: c.connections }),
              ]
                .filter(Boolean)
                .join(' · ')}
            </div>
          </div>
        )
      })}
    </StreamsBox>
  )
}
