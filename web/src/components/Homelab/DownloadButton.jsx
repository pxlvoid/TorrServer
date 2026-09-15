// homelab: "Download" next to "Preload" for every file of the torrent (hooks in DialogTorrentDetailsContent/Table).
// Preload buffers the start before playback; this puts the whole file into the persistent disk cache.
// While the file downloads the button fills up with its progress. Hidden unless the persistent cache is on.
import { Button, Tooltip, useTheme } from '@material-ui/core'
import { useTranslation } from 'react-i18next'

import './i18n'
import { downloadErrorText, homelabDownloadAction, jobHasFile, useHomelabDownload } from './downloads'

export default function HomelabDownloadButton({ hash, fileId }) {
  const { t } = useTranslation()
  const { palette } = useTheme()
  const data = useHomelabDownload(hash)
  if (!data?.enabled) return null

  const file = data.files?.find(f => f.id === fileId)
  const { job } = data
  const percent = file?.length ? Math.floor((file.done / file.length) * 100) : 0
  const complete = !!file?.length && file.done >= file.length
  const queued = !complete && jobHasFile(job, fileId)
  const failed = queued && job.state === 'error'
  // progress as a fill of the button, in its own color
  const fill = color => ({ background: `linear-gradient(to right, ${color}33 ${percent}%, transparent ${percent}%)` })

  let label = percent > 0 ? `${t('Homelab.Download')} · ${percent}%` : t('Homelab.Download')
  let hint = t('Homelab.DownloadFileHelp')
  let props = { color: 'primary', onClick: () => homelabDownloadAction(hash, 'start', [fileId]) }
  if (complete) {
    label = `✓ ${t('Homelab.FileOnDisk')}`
    hint = t('Homelab.EpisodeOnDisk')
    props = { color: 'primary', style: { ...fill(palette.primary.main), cursor: 'default' }, disableRipple: true }
  } else if (queued) {
    const active = job.state === 'active' && percent > 0
    label = failed ? t('Homelab.DownloadFailedShort') : active ? `↓ ${percent}%` : t('Homelab.DownloadQueuedShort')
    hint = [failed && downloadErrorText(t, job), t('Homelab.DownloadStopFile')].filter(Boolean).join('. ')
    props = {
      color: failed ? 'secondary' : 'primary',
      style: fill(palette.primary.main),
      onClick: () => homelabDownloadAction(hash, 'stop', [fileId]),
    }
  } else if (percent > 0) {
    props.style = fill(palette.primary.main)
  }

  return (
    <Tooltip title={hint}>
      <Button variant='outlined' size='small' {...props}>
        {label}
      </Button>
    </Tooltip>
  )
}
