// homelab: "Download to disk" in the torrent details, under the disk timeline (MiniCache): the job of the
// torrent — progress, speed, time left, why it can not run — and for a series a grid of episodes: what is
// on disk and what is downloading; a click queues an episode or takes it off the queue.
import { Button } from '@material-ui/core'
import GetAppIcon from '@material-ui/icons/GetApp'
import StopIcon from '@material-ui/icons/Stop'
import { useTranslation } from 'react-i18next'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'

import './i18n'
import { downloadErrorText, homelabDownloadAction, useHomelabDownload } from './downloads'
import EpisodeGrid, { episodesOf, useTimeLeft } from './EpisodeGrid'
import { DownloadBox, ProgressBar } from './style'

function Episodes({ hash, job, episodes, dark }) {
  const { t } = useTranslation()
  const onDisk = episodes.filter(e => e.done >= e.length).length
  return (
    <div className='dl-episodes'>
      <div className='dl-episodes-head'>
        {t('Homelab.EpisodesOnDisk')}: <b>{t('Homelab.EpisodesOf', { done: onDisk, count: episodes.length })}</b>
      </div>
      <EpisodeGrid hash={hash} job={job} episodes={episodes} dark={dark} />
    </div>
  )
}

export default function HomelabDownloadPanel({ hash, dark }) {
  const { t } = useTranslation()
  const timeLeft = useTimeLeft()
  const data = useHomelabDownload(hash)
  if (!data?.enabled) return null

  const { job, files = [], startError } = data
  const total = files.reduce((sum, f) => sum + f.length, 0)
  const done = files.reduce((sum, f) => sum + f.done, 0)
  const complete = total > 0 && done >= total
  const episodes = episodesOf(files)
  const jobPercent = job?.total ? Math.floor((job.done / job.total) * 100) : 0
  const stateLabel = {
    active: t('Homelab.Downloading'),
    queued: t('Homelab.DownloadQueued'),
    error: t('Homelab.DownloadFailedShort'),
  }

  let action = null
  if (job) {
    action = (
      <Button
        size='small'
        variant='outlined'
        startIcon={<StopIcon />}
        onClick={() => homelabDownloadAction(hash, 'stop')}
      >
        {t('Homelab.DownloadStop')}
      </Button>
    )
  } else if (!complete && total > 0) {
    action = (
      <Button
        size='small'
        variant='contained'
        color='secondary'
        disableElevation
        startIcon={<GetAppIcon />}
        onClick={() => homelabDownloadAction(hash, 'start')}
      >
        {done > 0 ? t('Homelab.DownloadRest') : t('Homelab.DownloadAll')}
      </Button>
    )
  }

  let summary
  if (complete) summary = <span className='dl-complete'>✓ {t('Homelab.AllOnDisk')}</span>
  else if (total > 0) summary = t('Homelab.DownloadLeft', { size: humanizeSize(total - done) })
  else summary = t('Homelab.DownloadHelp')

  return (
    <DownloadBox dark={dark}>
      <div className='dl-head'>
        <div className='dl-title'>
          <GetAppIcon />
          {t('Homelab.DownloadTitle')}
        </div>
        {action}
      </div>

      {job ? (
        <>
          <div className='dl-status'>
            <span className={`dl-state dl-state-${job.state}`}>
              {stateLabel[job.state]}
              {job.files?.length > 0 && ` · ${t('Homelab.DownloadFiles', { count: job.files.length })}`}
            </span>
            {job.total > 0 && <span className='dl-percent'>{jobPercent}%</span>}
          </div>
          <ProgressBar dark={dark} value={jobPercent} active={job.state === 'active'} />
          {job.state === 'error' ? (
            <div className='dl-meta dl-error'>{downloadErrorText(t, job)}</div>
          ) : (
            <div className='dl-meta'>
              {[
                job.total > 0 &&
                  t('Homelab.SizeOf', { size: humanizeSize(job.done) || '0', total: humanizeSize(job.total) }),
                job.state === 'active' && job.speed > 0 && humanizeSpeed(job.speed),
                job.state === 'active' && job.speed > 0 && timeLeft((job.total - job.done) / job.speed),
              ]
                .filter(Boolean)
                .join(' · ')}
            </div>
          )}
        </>
      ) : (
        <div className='dl-meta'>{summary}</div>
      )}

      {startError && <div className='dl-meta dl-error'>{downloadErrorText(t, startError)}</div>}

      {episodes.length > 1 && <Episodes hash={hash} job={job} episodes={episodes} dark={dark} />}
    </DownloadBox>
  )
}
