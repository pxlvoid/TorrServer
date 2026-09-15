// homelab: the audio track served to players (hook in DialogTorrentDetailsContent; server: torr/homelab_audio.go).
// The Lampa player plays the first or the default track of an MKV and can not switch; with a track chosen here
// the others are hidden in the stream. A series may be dubbed by different studios in different episodes, so
// there is a main track and a fallback for episodes without it, and a summary of what each episode will play;
// an episode can also get its own track in the "Audio" column of the file table (reset here).
// Shown only for MKV files with several audio tracks. Data: the shared poller of audio.js (the file table has
// an "Audio" column on the same data).
import { Button, FormControl, InputLabel, MenuItem, Select, Typography } from '@material-ui/core'
import { SmallLabel } from 'components/DialogTorrentDetailsContent/TorrentFunctions/style'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import './i18n'
import { homelabAudioClearFiles, homelabAudioSave, trackKey, useHomelabAudio } from './audio'
import { labelEpisodes } from './episodes'
import { AudioBox } from './style'

const ALL = 'all'
const SAME_LANG = 'lang'

// distinct tracks over all episodes: label, language and in how many episodes they are
function distinctTracks(files) {
  const map = new Map()
  files.forEach(f =>
    f.tracks.forEach(track => {
      const key = trackKey(track)
      const entry = map.get(key) || { key, name: track.name, lang: track.lang, codec: track.codec, count: 0 }
      entry.count += 1
      map.set(key, entry)
    }),
  )
  return [...map.values()].sort((a, b) => b.count - a.count)
}

// consecutive episodes playing the same thing: "1–3 LostFilm", "4–8 HDRezka"
function summary(files) {
  const groups = []
  labelEpisodes(files).forEach(f => {
    const track = f.picked >= 0 ? f.tracks[f.picked] : null
    let key = 'none'
    if (!f.ready) key = 'wait'
    else if (track) key = `${trackKey(track)}|${f.how === 'file' ? 'own' : ''}`
    const last = groups[groups.length - 1]
    if (last && last.key === key) last.to = f.label
    else groups.push({ key, from: f.label, to: f.label, track, how: f.how })
  })
  return groups
}

export default function HomelabAudioSelect({ hash }) {
  const { t } = useTranslation()
  const data = useHomelabAudio(hash)
  const [error, setError] = useState('')

  const files = data?.files || []
  const tracks = distinctTracks(files.filter(f => f.ready))
  if (!files.some(f => f.tracks.length > 1)) return null

  const ready = files.filter(f => f.ready).length
  const prefs = data.choice?.tracks || []
  const main = prefs[0] ? tracks.find(tr => tr.key === trackKey(prefs[0])) : null
  const fallback = prefs[1] ? tracks.find(tr => tr.key === trackKey(prefs[1])) : null
  const series = files.length > 1
  const own = files.filter(f => f.how === 'file').length // episodes with their own track (the file table)

  const save = next =>
    homelabAudioSave(hash, next)
      .then(() => setError(''))
      .catch(err => setError(err.message))

  const option = track => (
    <MenuItem key={track.key} value={track.key}>
      {[track.name || t('Homelab.AudioNoName'), track.lang?.toUpperCase(), track.codec].filter(Boolean).join(' · ')}
      {series && track.count < ready && (
        <span className='audio-note'>{t('Homelab.AudioInEpisodes', { count: track.count, total: ready })}</span>
      )}
    </MenuItem>
  )

  const describe = group => {
    const episodes =
      group.from === group.to
        ? t('Homelab.AudioEpisode', { n: group.from })
        : t('Homelab.AudioEpisodes', { from: group.from, to: group.to })
    if (group.key === 'wait') return `${episodes} — ${t('Homelab.AudioNotReady')}`
    if (!group.track) return `${episodes} — ${t('Homelab.AudioAsFile')}`
    let how = ''
    if (group.how === 'lang') how = ` (${t('Homelab.AudioByLang')})`
    if (group.how === 'file') how = ` (${t('Homelab.AudioOwn')})`
    return `${episodes} — ${group.track.name || t('Homelab.AudioNoName')}${how}`
  }

  return (
    <AudioBox>
      <SmallLabel mb={14}>{t('Homelab.AudioTitle')}</SmallLabel>
      <div className='audio-selects'>
        <FormControl variant='outlined' size='small' fullWidth>
          <InputLabel id={`audio-main-${hash}`}>{t('Homelab.AudioMain')}</InputLabel>
          <Select
            labelId={`audio-main-${hash}`}
            label={t('Homelab.AudioMain')}
            value={main ? main.key : ALL}
            onChange={e => {
              const track = tracks.find(tr => tr.key === e.target.value)
              // the new main keeps the fallback unless it is the fallback itself
              save(track ? [track, ...(fallback && fallback.key !== track.key ? [fallback] : [])] : [])
            }}
          >
            <MenuItem value={ALL}>{t('Homelab.AudioAll')}</MenuItem>
            {tracks.map(option)}
          </Select>
        </FormControl>

        {main && series && main.count < ready && (
          <FormControl variant='outlined' size='small' fullWidth>
            <InputLabel id={`audio-fallback-${hash}`}>{t('Homelab.AudioFallback')}</InputLabel>
            <Select
              labelId={`audio-fallback-${hash}`}
              label={t('Homelab.AudioFallback')}
              value={fallback ? fallback.key : SAME_LANG}
              onChange={e => {
                const track = tracks.find(tr => tr.key === e.target.value)
                save(track ? [main, track] : [main])
              }}
            >
              <MenuItem value={SAME_LANG}>
                {t('Homelab.AudioSameLang', { lang: main.lang?.toUpperCase() || '—' })}
              </MenuItem>
              {tracks.filter(tr => tr.key !== main.key).map(option)}
            </Select>
          </FormControl>
        )}
      </div>

      {(main || own > 0) && series && (
        <ul className='audio-summary'>
          {summary(files).map(group => (
            <li key={`${group.from}-${group.key}`} className={group.track ? '' : 'audio-summary-muted'}>
              {describe(group)}
            </li>
          ))}
        </ul>
      )}

      {own > 0 && (
        <div className='audio-own'>
          {t('Homelab.AudioOwnCount', { count: own })}
          <Button size='small' onClick={() => homelabAudioClearFiles(hash).catch(err => setError(err.message))}>
            {t('Homelab.AudioOwnReset')}
          </Button>
        </div>
      )}

      <Typography variant='body2' color={error ? 'error' : 'textSecondary'} className='audio-help'>
        {error || (main || own > 0 ? t('Homelab.AudioHelpOn') : t('Homelab.AudioHelpOff'))}
      </Typography>
    </AudioBox>
  )
}
