// homelab: a grid of the episodes of a series — what is on disk, what is downloading, what is queued; a click queues
// an episode or takes it off the queue. Used by the download panel of the torrent details and the disk cache dialog.
import { Tooltip } from '@material-ui/core'
import { useTranslation } from 'react-i18next'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import { homelabDownloadAction, jobHasFile } from './downloads'
import { labelEpisodes, VIDEO_EXT } from './episodes'
import { EpisodeChip, EpisodeGridBox } from './style'

// video files with their episode labels
export const episodesOf = files => labelEpisodes((files || []).filter(f => VIDEO_EXT.test(f.path)))

// the server takes the files of a job in order: the first unfinished one is being downloaded
export const currentEpisode = (job, episodes) =>
  job?.state === 'active' ? episodes.find(e => jobHasFile(job, e.id) && e.done < e.length) || null : null

// "~12 min left" / "~2 h 5 min left"
export function useTimeLeft() {
  const { t } = useTranslation()
  return seconds => {
    if (!Number.isFinite(seconds) || seconds <= 0) return ''
    const minutes = Math.max(1, Math.round(seconds / 60))
    const time =
      minutes < 60
        ? t('Homelab.Minutes', { count: minutes })
        : t('Homelab.HoursMinutes', { h: Math.floor(minutes / 60), m: minutes % 60 })
    return t('Homelab.TimeLeft', { time })
  }
}

export default function EpisodeGrid({ hash, job, episodes, dark }) {
  const { t } = useTranslation()
  const current = currentEpisode(job, episodes)

  return (
    <EpisodeGridBox>
      {episodes.map(e => {
        const percent = e.length ? Math.floor((e.done / e.length) * 100) : 0
        const complete = e.done >= e.length
        const queued = jobHasFile(job, e.id) && !complete
        const state = complete ? 'done' : e === current ? 'active' : queued ? 'queued' : 'none'
        const hint = complete
          ? t('Homelab.EpisodeOnDisk')
          : queued
          ? t('Homelab.EpisodeClickStop')
          : t('Homelab.EpisodeClickDownload')
        const title = [
          t('Homelab.Episode', { n: e.label }),
          humanizeSize(e.length),
          t('Homelab.PercentOnDisk', { percent }),
        ]
        return (
          <Tooltip key={e.id} title={`${title.join(' · ')}. ${hint}`}>
            {/* span: a disabled button gets no mouse events, the tooltip still has to work */}
            <span>
              <EpisodeChip
                type='button'
                dark={dark}
                state={state}
                percent={percent}
                disabled={complete}
                onClick={ev => {
                  ev.stopPropagation()
                  homelabDownloadAction(hash, queued ? 'stop' : 'start', [e.id])
                }}
              >
                {e.label}
              </EpisodeChip>
            </span>
          </Tooltip>
        )
      })}
    </EpisodeGridBox>
  )
}
