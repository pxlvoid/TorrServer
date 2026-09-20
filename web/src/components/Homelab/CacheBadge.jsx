// homelab: "on disk" badge on a torrent card poster (hook in TorrentCard): share of the torrent in the
// disk cache — for a series episodes completely on disk, "3/8" — (with an arrow while it is being downloaded),
// "▶ 18 Mbit/s" at the bottom while someone watches it (streams.js).
// Nothing on disk and nobody watching — nothing shown.
import GetAppIcon from '@material-ui/icons/GetApp'
import StorageIcon from '@material-ui/icons/Storage'
import { useTranslation } from 'react-i18next'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'

import './i18n'
import { useHomelabCache } from './store'
import { useTorrentStreams } from './streams'
import { LiveMark, PosterBadge } from './style'

// who watches the torrent: "▶ 18 Mbit/s", "▶ 2 · 36 Mbit/s", "▶ paused"
function Live({ hash }) {
  const { t } = useTranslation()
  const clients = useTorrentStreams(hash).filter(c => !c.ended)
  if (!clients.length) return null
  const speed = clients.reduce((sum, c) => sum + c.speed, 0)
  const label = [clients.length > 1 && clients.length, speed > 0 ? humanizeSpeed(speed) : t('Homelab.StreamPaused')]
    .filter(Boolean)
    .join(' · ')
  return <LiveMark title={clients.map(c => c.device).join(', ')}>▶ {label}</LiveMark>
}

export default function HomelabCacheBadge({ hash }) {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const item = data?.items?.find(it => it.hash === hash)
  const download = data?.downloads?.find(d => d.hash === hash)
  if (!item || (!item.size && !download)) return <Live hash={hash} />

  const percent = item.totalLength ? Math.min(100, (item.size / item.totalLength) * 100) : null
  const series = item.episodes > 1
  const label =
    download?.state === 'queued'
      ? t('Homelab.DownloadQueuedShort')
      : series
      ? `${item.episodesDone}/${item.episodes}`
      : percent === null
      ? humanizeSize(item.size) || '0'
      : percent >= 99.5
      ? t('Homelab.BadgeFull')
      : `${Math.max(1, Math.round(percent))}%`
  const title = [
    download
      ? t('Homelab.BadgeDownloading', { size: humanizeSize(item.size) || '0', total: humanizeSize(item.totalLength) })
      : t('Homelab.BadgeTitle', { size: humanizeSize(item.size) }),
    series && t('Homelab.BadgeEpisodes', { done: item.episodesDone, count: item.episodes }),
  ]
    .filter(Boolean)
    .join('. ')

  return (
    <>
      <PosterBadge title={title}>
        <div className='badge-label'>
          {download ? <GetAppIcon /> : <StorageIcon />}
          {label}
        </div>
        {percent !== null && (
          <div className='badge-bar'>
            <div style={{ width: `${Math.max(2, percent)}%` }} />
          </div>
        )}
      </PosterBadge>
      <Live hash={hash} />
    </>
  )
}
