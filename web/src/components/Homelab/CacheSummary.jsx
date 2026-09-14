// homelab: disk cache summary above the torrent list (hook in TorrentList). Click opens the disk cache dialog.
import StorageIcon from '@material-ui/icons/Storage'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import DiskCacheDialog from './DiskCacheDialog'
import { parseTitle } from './parseTitle'
import { useHomelabCache } from './store'
import { Summary, SummaryBar } from './style'

export default function HomelabCacheSummary() {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  if (!data?.usage?.ready) return null
  const { usage, items = [], settings } = data
  const playing = items.filter(item => item.playing)
  const used = humanizeSize(usage.used) || '0'

  return (
    <>
      <Summary onClick={() => setIsDialogOpen(true)} title={t('Homelab.OpenDialog')}>
        <StorageIcon className='summary-icon' />
        <div className='summary-main'>
          {t('Homelab.DiskCache')}:{' '}
          <b>{usage.limit ? t('Homelab.Used', { used, limit: humanizeSize(usage.limit) }) : used}</b>
          {!settings?.persistentCache && ` · ${t('Homelab.SummaryOff')}`}
        </div>
        <div className='summary-side'>
          {[
            t('Homelab.SummaryTorrents', { count: items.length }),
            usage.diskTotal && t('Homelab.DiskFree', { free: humanizeSize(usage.diskFree) }),
          ]
            .filter(Boolean)
            .join(' · ')}
        </div>
        {!!usage.limit && (
          <div className='summary-bar'>
            <SummaryBar value={(usage.used / usage.limit) * 100} />
          </div>
        )}
        {playing.length > 0 && (
          <div className='summary-playing'>
            ▶ {t('Homelab.Playing')}:{' '}
            {playing.map(item => parseTitle(item.title || item.name || item.hash).title).join(', ')}
          </div>
        )}
      </Summary>

      {isDialogOpen && <DiskCacheDialog handleClose={() => setIsDialogOpen(false)} />}
    </>
  )
}
