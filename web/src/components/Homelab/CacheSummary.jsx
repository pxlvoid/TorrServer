// homelab: disk cache summary above the torrent list (hook in TorrentList). Click opens the disk cache dialog.
// Under it, a card of its own — who is watching now (StreamsSummary): shown whatever the disk cache.
import StorageIcon from '@material-ui/icons/Storage'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import DiskCacheDialog from './DiskCacheDialog'
import { parseTitle } from './parseTitle'
import { useHomelabCache } from './store'
import HomelabStreamsSummary from './StreamsSummary'
import { Summary, SummaryBar } from './style'

export default function HomelabCacheSummary() {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  if (!data?.usage?.ready) return <HomelabStreamsSummary />
  const { usage, items = [], settings, downloads = [] } = data
  const titleOf = hash => {
    const item = items.find(it => it.hash === hash)
    return parseTitle(item?.title || item?.name || hash).title
  }
  const active = downloads.find(d => d.state === 'active')
  const used = humanizeSize(usage.used) || '0'

  return (
    <>
      <Summary onClick={() => setIsDialogOpen(true)} title={t('Homelab.OpenDialog')}>
        <StorageIcon className='summary-icon' />
        <div className='summary-main'>
          {t('Homelab.DiskCache')}:{' '}
          <b>{usage.limit ? t('Homelab.Used', { used, limit: humanizeSize(usage.limit) }) : used}</b>
          {/* "use disk" off wins over the persistent switch: nothing is written either way, and this is why */}
          {!usage.useDisk
            ? ` · ${t('Homelab.SummaryFrozen')}`
            : !settings?.persistentCache && ` · ${t('Homelab.SummaryOff')}`}
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
        {downloads.length > 0 && (
          <div className='summary-playing'>
            ↓ {t('Homelab.SummaryDownloading')}:{' '}
            {[
              active &&
                `${titleOf(active.hash)}${active.total ? ` ${Math.floor((active.done / active.total) * 100)}%` : ''}`,
              downloads.length > (active ? 1 : 0) &&
                `${t('Homelab.DownloadQueued').toLowerCase()} ${downloads.length - (active ? 1 : 0)}`,
            ]
              .filter(Boolean)
              .join(' · ')}
          </div>
        )}
      </Summary>

      <HomelabStreamsSummary />

      {isDialogOpen && <DiskCacheDialog handleClose={() => setIsDialogOpen(false)} />}
    </>
  )
}
