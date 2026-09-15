// homelab: disk cache summary above the torrent list (hook in TorrentList). Click opens the disk cache dialog.
// Also who is watching now: clients of the stream with the device, what, how fast (streams.js).
import StorageIcon from '@material-ui/icons/Storage'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'

import './i18n'
import DiskCacheDialog from './DiskCacheDialog'
import { parseTitle } from './parseTitle'
import { useHomelabCache } from './store'
import { episodeOf, useHomelabStreams } from './streams'
import { Summary, SummaryBar } from './style'

export default function HomelabCacheSummary() {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const clients = useHomelabStreams()
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  if (!data?.usage?.ready) return null
  const { usage, items = [], settings, downloads = [] } = data
  const playing = items.filter(item => item.playing)
  const titleOf = hash => {
    const item = items.find(it => it.hash === hash)
    return parseTitle(item?.title || item?.name || hash).title
  }
  const active = downloads.find(d => d.state === 'active')
  const used = humanizeSize(usage.used) || '0'
  const watching = (clients || []).filter(c => !c.ended)
  const totalSpeed = watching.reduce((sum, c) => sum + c.speed, 0)
  const clientLine = c =>
    [
      `${c.device} — ${parseTitle(c.title || c.path).title}`,
      episodeOf(c.path),
      c.active ? humanizeSpeed(c.speed) : t('Homelab.StreamPaused'),
      c.position > 0 && `${Math.round(c.position * 100)}%`,
    ]
      .filter(Boolean)
      .join(' · ')

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
        {clients && watching.length > 0 && (
          <>
            <div className='summary-playing'>
              ▶ {t('Homelab.StreamsWatching', { count: watching.length })}
              {totalSpeed > 0 && ` · ${humanizeSpeed(totalSpeed)}`}
            </div>
            {watching.map(c => (
              <div key={`${c.ip}|${c.ua}|${c.hash}|${c.path}`} className='summary-playing summary-client'>
                {clientLine(c)}
              </div>
            ))}
          </>
        )}
        {!clients && playing.length > 0 && (
          <div className='summary-playing'>
            ▶ {t('Homelab.Playing')}:{' '}
            {playing.map(item => parseTitle(item.title || item.name || item.hash).title).join(', ')}
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

      {isDialogOpen && <DiskCacheDialog handleClose={() => setIsDialogOpen(false)} />}
    </>
  )
}
