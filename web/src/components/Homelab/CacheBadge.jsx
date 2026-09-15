// homelab: "on disk" badge on a torrent card poster (hook in TorrentCard): share of the torrent in the
// disk cache — for a series episodes completely on disk, "3/8" — (with an arrow while it is being downloaded),
// and a star if it is pinned. Nothing on disk — nothing shown.
import GetAppIcon from '@material-ui/icons/GetApp'
import StorageIcon from '@material-ui/icons/Storage'
import StarIcon from '@material-ui/icons/Star'
import { useTranslation } from 'react-i18next'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import { useHomelabCache } from './store'
import { PinMark, PosterBadge } from './style'

export default function HomelabCacheBadge({ hash }) {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const item = data?.items?.find(it => it.hash === hash)
  const download = data?.downloads?.find(d => d.hash === hash)
  if (!item || (!item.size && !download)) return null

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
      {item.pinned && (
        <PinMark>
          <StarIcon />
        </PinMark>
      )}
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
    </>
  )
}
