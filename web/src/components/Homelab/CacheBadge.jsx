// homelab: "on disk" badge on a torrent card poster (hook in TorrentCard): share of the torrent in the
// disk cache, and a star if it is pinned. Nothing on disk — nothing shown.
import StorageIcon from '@material-ui/icons/Storage'
import StarIcon from '@material-ui/icons/Star'
import { useTranslation } from 'react-i18next'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import { useHomelabItem } from './store'
import { PinMark, PosterBadge } from './style'

export default function HomelabCacheBadge({ hash }) {
  const { t } = useTranslation()
  const item = useHomelabItem(hash)
  if (!item || !item.size) return null

  const percent = item.totalLength ? Math.min(100, (item.size / item.totalLength) * 100) : null
  const label =
    percent === null
      ? humanizeSize(item.size)
      : percent >= 99.5
      ? t('Homelab.BadgeFull')
      : `${Math.max(1, Math.round(percent))}%`

  return (
    <>
      {item.pinned && (
        <PinMark>
          <StarIcon />
        </PinMark>
      )}
      <PosterBadge title={t('Homelab.BadgeTitle', { size: humanizeSize(item.size) })}>
        <div className='badge-label'>
          <StorageIcon />
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
