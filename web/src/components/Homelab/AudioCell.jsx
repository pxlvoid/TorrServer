// homelab: the "Audio" column of the file table (hooks in DialogTorrentDetailsContent/Table): what the player gets
// in each episode, and a menu of the episode's tracks — a click sets the track for this episode only (it wins
// over the torrent's choice under "Info"; "As in the whole torrent" takes it back). Same data as that choice
// (audio.js). The column is there only if some file has several audio tracks.
import { Button, Divider, Menu, MenuItem, Tooltip } from '@material-ui/core'
import ArrowDropDownIcon from '@material-ui/icons/ArrowDropDown'
import CheckIcon from '@material-ui/icons/Check'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import './i18n'
import { homelabAudioSetFile, playingTrack, useHomelabAudio } from './audio'
import { AudioPicker } from './style'

const shown = data => !!data?.files?.some(f => f.tracks.length > 1)

const trackLine = (t, track) =>
  [track.name || t('Homelab.AudioNoName'), track.lang?.toUpperCase(), track.codec].filter(Boolean).join(' · ')

function Picker({ hash, fileId }) {
  const { t } = useTranslation()
  const data = useHomelabAudio(hash)
  const [anchor, setAnchor] = useState(null)
  const file = data?.files?.find(f => f.id === fileId)
  if (!file) return null
  if (!file.ready) {
    return <span className='audio-muted'>{t('Homelab.AudioReading')}</span>
  }
  const playing = playingTrack(file)
  const label = playing ? playing.name || playing.lang?.toUpperCase() || t('Homelab.AudioNoName') : '—'
  if (file.tracks.length < 2) {
    return <span className='audio-muted'>{label}</span>
  }

  // why this track: the episode's own, the torrent's choice, the same language, or the file as is
  const own = file.how === 'file'
  let hint = t('Homelab.AudioChosenHint')
  let tag = null
  if (own) {
    hint = t('Homelab.AudioOwnHint')
    tag = t('Homelab.AudioOwnShort')
  } else if (file.how === 'lang') {
    hint = t('Homelab.AudioByLangHint')
    tag = t('Homelab.AudioByLangShort')
  } else if (file.picked < 0) {
    hint = t('Homelab.AudioFileDefaultHint')
    tag = t('Homelab.AudioDefaultShort')
  }

  const choose = track => {
    setAnchor(null)
    homelabAudioSetFile(hash, file.path, track).catch(() => {})
  }

  return (
    <>
      <Tooltip title={`${hint}. ${t('Homelab.AudioClickHint')}`}>
        <Button
          size='small'
          className={file.picked >= 0 ? 'audio-button audio-chosen' : 'audio-button'}
          endIcon={<ArrowDropDownIcon />}
          onClick={e => setAnchor(e.currentTarget)}
        >
          <span className='audio-label'>{label}</span>
          {tag && <span className={own ? 'audio-tag audio-tag-own' : 'audio-tag'}>{tag}</span>}
        </Button>
      </Tooltip>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
        <MenuItem disabled dense>
          {t('Homelab.AudioMenuTitle')}
        </MenuItem>
        {file.tracks.map(track => {
          const isPlaying = playing?.index === track.index
          return (
            <MenuItem key={track.index} dense selected={isPlaying} onClick={() => choose(track)}>
              <span className='audio-check'>{isPlaying && <CheckIcon fontSize='small' />}</span>
              {trackLine(t, track)}
            </MenuItem>
          )
        })}
        {own && <Divider />}
        {own && (
          <MenuItem dense onClick={() => choose(null)}>
            <span className='audio-check' />
            {t('Homelab.AudioBackToTorrent')}
          </MenuItem>
        )}
      </Menu>
    </>
  )
}

export function HomelabAudioTh({ hash }) {
  const { t } = useTranslation()
  const data = useHomelabAudio(hash)
  return shown(data) ? <th style={{ width: '220px' }}>{t('Homelab.AudioColumn')}</th> : null
}

export function HomelabAudioTd({ hash, fileId }) {
  const data = useHomelabAudio(hash)
  if (!shown(data)) return null
  return (
    <td data-label='audio'>
      <AudioPicker>
        <Picker hash={hash} fileId={fileId} />
      </AudioPicker>
    </td>
  )
}

// the short (mobile) table: a field like "Size"
export function HomelabAudioShort({ hash, fileId }) {
  const { t } = useTranslation()
  const data = useHomelabAudio(hash)
  if (!shown(data)) return null
  return (
    <div className='short-table-field'>
      <div className='short-table-field-name'>{t('Homelab.AudioColumn')}</div>
      <div className='short-table-field-value'>
        <AudioPicker>
          <Picker hash={hash} fileId={fileId} />
        </AudioPicker>
      </div>
    </div>
  )
}
