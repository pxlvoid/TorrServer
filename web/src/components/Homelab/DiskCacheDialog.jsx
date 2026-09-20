// homelab: what the persistent disk cache holds — usage, settings, per-torrent download / remove.
// Accent is "secondary": in the dark theme TorrServer's primary (#323637) is the dialog background.
import axios from 'axios'
import {
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  Switch,
  TextField,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from '@material-ui/core'
import DeleteIcon from '@material-ui/icons/Delete'
import GetAppIcon from '@material-ui/icons/GetApp'
import MovieIcon from '@material-ui/icons/Movie'
import StopIcon from '@material-ui/icons/Stop'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { StyledDialog } from 'style/CustomMaterialUiStyles'
import { getTorrServerHost } from 'utils/Hosts'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'
import useOnStandaloneAppOutsideClick from 'utils/useOnStandaloneAppOutsideClick'

import UnsafeButton from '../UnsafeButton'
import { downloadErrorText, homelabDownloadAction, jobHasFile, useHomelabDownload } from './downloads'
import EpisodeGrid, { currentEpisode, episodesOf, useTimeLeft } from './EpisodeGrid'
import HomelabNtfyForm, { NtfySummary } from './NtfySettings'
import HomelabTorrentInfoDialog from './TorrentInfoDialog'
import { parseTitle } from './parseTitle'
import { publishHomelabCache } from './store'
import {
  Actions,
  Bar,
  Card,
  CardList,
  Info,
  PlayingBadge,
  Poster,
  Section,
  SettingsForm,
  SettingsRow,
  UsageRow,
} from './style'

const cacheHost = () => `${getTorrServerHost()}/homelab/cache`
const settingsHost = () => `${getTorrServerHost()}/homelab/settings`
const REFRESH_MS = 5000
const CONFIRM_MS = 5000

const errorText = err => err?.response?.data?.error || err?.message || String(err)

function useRelativeTime() {
  const { i18n } = useTranslation()
  return useCallback(
    unix => {
      if (!unix) return ''
      const rtf = new Intl.RelativeTimeFormat(i18n.language, { numeric: 'auto' })
      const diff = unix - Date.now() / 1000
      const steps = [
        [60, 'second'],
        [3600, 'minute'],
        [86400, 'hour'],
        [86400 * 30, 'day'],
        [Infinity, 'month'],
      ]
      let unit = 'second'
      let div = 1
      for (const [limit, name] of steps) {
        unit = name
        if (Math.abs(diff) < limit) break
        div = limit
      }
      return rtf.format(Math.round(diff / div), unit)
    },
    [i18n.language],
  )
}

// the download of a torrent, short: which episode, how fast, how long is left, how many more are queued
function CardDownload({ hash, download }) {
  const { t } = useTranslation()
  const timeLeft = useTimeLeft()
  const data = useHomelabDownload(hash) // shared with the episode grid: the files and what of them is on disk
  const job = data?.job || download
  if (!job) return null
  if (job.state === 'error') {
    return (
      <Tooltip title={downloadErrorText(t, job)}>
        <span className='card-download card-download-error'>
          ↓ {t(`Homelab.DownloadErrorShort.${job.error}`, { defaultValue: t('Homelab.DownloadErrorShort.failed') })}
        </span>
      </Tooltip>
    )
  }
  if (job.state === 'queued') return <span className='card-download'>↓ {t('Homelab.DownloadQueued')}</span>

  const episodes = episodesOf(data?.files)
  const current = episodes.length > 1 ? currentEpisode(job, episodes) : null
  const more = episodes.filter(e => e !== current && jobHasFile(job, e.id) && e.done < e.length).length
  const parts = [
    current ? t('Homelab.DownloadingEpisodeShort', { n: current.label }) : t('Homelab.Downloading'),
    job.speed > 0 && humanizeSpeed(job.speed),
    job.speed > 0 && job.total > job.done && timeLeft((job.total - job.done) / job.speed),
    more > 0 && t('Homelab.DownloadQueuedMore', { count: more }),
  ]
  return <span className='card-download'>↓ {parts.filter(Boolean).join(' · ')}</span>
}

// the episodes of a series on disk — a grid, a click queues an episode or takes it off the queue
function CardEpisodes({ hash, dark }) {
  const { t } = useTranslation()
  const data = useHomelabDownload(hash)
  const episodes = episodesOf(data?.files)
  if (!data) return <div className='card-episodes-wait'>{t('Homelab.EpisodesLoading')}</div>
  if (!episodes.length) return null
  return (
    <div className='card-episodes'>
      <EpisodeGrid hash={hash} job={data.job} episodes={episodes} dark={dark} />
    </div>
  )
}

function CacheCard({ item, download, confirming, busy, dark, onRemove, onDownload, onStop, onOpen }) {
  const { t } = useTranslation()
  const relativeTime = useRelativeTime()
  const [posterFailed, setPosterFailed] = useState(false)
  const [episodesOpen, setEpisodesOpen] = useState(false)
  const series = item.episodes > 1
  const fullName = item.title || item.name || item.hash
  const { title, subtitle } = parseTitle(fullName)
  const percent = item.totalLength ? (item.size / item.totalLength) * 100 : null
  const complete = !!item.totalLength && item.size >= item.totalLength
  const badge = item.playing
    ? t('Homelab.PlayingShort')
    : download?.state === 'active'
    ? t('Homelab.DownloadingShort')
    : ''
  const stats = [
    item.episodes > 1 && t('Homelab.CardEpisodes', { done: item.episodesDone, count: item.episodes }),
    item.totalLength
      ? t('Homelab.SizeOf', { size: humanizeSize(item.size), total: humanizeSize(item.totalLength) })
      : humanizeSize(item.size),
    relativeTime(item.lastAccess),
  ].filter(Boolean)

  const action = (tip, icon, onClick, extra = {}) => (
    <Tooltip title={tip}>
      <span>
        <IconButton size='small' disabled={busy} onClick={onClick} {...extra}>
          {icon}
        </IconButton>
      </span>
    </Tooltip>
  )

  return (
    <Card dark={dark}>
      <Poster onClick={() => onOpen(item)} title={t('Homelab.OpenInfo')} style={{ cursor: 'pointer' }}>
        {item.poster && !posterFailed ? (
          <img src={item.poster} alt='' loading='lazy' onError={() => setPosterFailed(true)} />
        ) : (
          <MovieIcon />
        )}
        {badge && (
          <PlayingBadge dark={dark} title={item.playing ? t('Homelab.Playing') : t('Homelab.Downloading')}>
            {badge}
          </PlayingBadge>
        )}
      </Poster>

      <Info>
        <div className='card-head'>
          <button type='button' className='card-title card-open' onClick={() => onOpen(item)} title={fullName}>
            {title}
          </button>
          <Actions>
            {download
              ? action(t('Homelab.DownloadStop'), <StopIcon fontSize='small' />, () => onStop(item))
              : !complete &&
                action(t('Homelab.DownloadAllHelp'), <GetAppIcon fontSize='small' />, () => onDownload(item))}
            {action(
              confirming ? t('Homelab.RemoveConfirm') : item.playing ? t('Homelab.RemovePlaying') : t('Homelab.Remove'),
              <DeleteIcon fontSize='small' />,
              () => onRemove(item),
              { color: confirming ? 'secondary' : 'default' },
            )}
          </Actions>
        </div>
        {subtitle && <div className='card-subtitle'>{subtitle}</div>}
        {percent !== null && <Bar thin dark={dark} value={percent} className='card-bar' />}
        <div className='card-line'>{stats.join(' · ')}</div>
        {(download || series) && (
          <div className='card-line card-line-split'>
            {download ? <CardDownload hash={item.hash} download={download} /> : <span />}
            {series && (
              <button type='button' className='card-toggle' onClick={() => setEpisodesOpen(!episodesOpen)}>
                {episodesOpen ? t('Homelab.EpisodesHide') : t('Homelab.EpisodesShow')}
              </button>
            )}
          </div>
        )}
        {series && episodesOpen && <CardEpisodes hash={item.hash} dark={dark} />}
      </Info>
    </Card>
  )
}

export default function DiskCacheDialog({ handleClose }) {
  const { t } = useTranslation()
  const fullScreen = useMediaQuery('@media (max-width:930px)')
  const dark = useTheme().palette.type === 'dark'
  const ref = useOnStandaloneAppOutsideClick(handleClose)

  const [data, setData] = useState(null)
  const [form, setForm] = useState(null)
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [confirmHash, setConfirmHash] = useState('') // playing torrent: the second click removes
  const [infoHash, setInfoHash] = useState('') // the standard "Torrent info" window over this one

  useEffect(() => {
    if (!confirmHash) return undefined
    const id = setTimeout(() => setConfirmHash(''), CONFIRM_MS)
    return () => clearTimeout(id)
  }, [confirmHash])

  const apply = useCallback(
    resp => {
      setData(resp)
      publishHomelabCache(resp) // summary and badges on the main screen follow the dialog
      setForm(prev => prev || resp.settings)
      if (resp.freed) setMessage(t('Homelab.Freed', { size: humanizeSize(resp.freed) }))
    },
    [t],
  )

  const call = useCallback(
    (body, withBusy = true) => {
      if (withBusy) setBusy(true)
      return axios
        .post(cacheHost(), body)
        .then(({ data }) => {
          apply(data)
          setError('')
        })
        .catch(err => setError(errorText(err)))
        .finally(() => withBusy && setBusy(false))
    },
    [apply],
  )

  useEffect(() => {
    call({ action: 'list' }, false)
    const id = setInterval(() => call({ action: 'list' }, false), REFRESH_MS)
    return () => clearInterval(id)
  }, [call])

  const removeItem = item => {
    if (item.playing && confirmHash !== item.hash) {
      setConfirmHash(item.hash)
      setMessage(t('Homelab.RemoveConfirm'))
      return
    }
    setConfirmHash('')
    call({ action: 'remove', hash: item.hash })
  }

  const downloadItem = (item, action) => {
    setBusy(true)
    homelabDownloadAction(item.hash, action)
      .then(res => {
        setError(res?.startError ? downloadErrorText(t, res.startError) : '')
        if (!res?.startError) setMessage(action === 'start' ? t('Homelab.DownloadStarted') : '')
        return call({ action: 'list' }, false)
      })
      .finally(() => setBusy(false))
  }

  const saveSettings = sets => {
    setBusy(true)
    axios
      .post(settingsHost(), {
        persistentCache: sets.persistentCache,
        limitGB: Math.max(0, parseInt(sets.limitGB, 10) || 0),
        keepDays: Math.max(0, parseInt(sets.keepDays, 10) || 0),
        backgroundFill: !!sets.backgroundFill, // the API stores the whole struct: never omit a field
        nextEpisode: !!sets.nextEpisode,
      })
      .then(({ data }) => {
        setForm(data)
        setEditing(false)
        setMessage(t('Homelab.Saved'))
        setError('')
        return call({ action: 'list' }, false)
      })
      .catch(err => setError(errorText(err)))
      .finally(() => setBusy(false))
  }

  const usage = data?.usage
  const saved = data?.settings
  const downloads = data?.downloads || []
  const rank = it => {
    const d = downloads.find(x => x.hash === it.hash)
    if (it.playing) return 0
    if (d?.state === 'active') return 1
    return d ? 2 : 3
  }
  const items = (data?.items || [])
    .map((it, i) => ({ it, i }))
    .sort((a, b) => rank(a.it) - rank(b.it) || a.i - b.i)
    .map(x => x.it)
  const dirty =
    form &&
    saved &&
    (String(form.limitGB) !== String(saved.limitGB) ||
      String(form.keepDays) !== String(saved.keepDays) ||
      !!form.backgroundFill !== !!saved.backgroundFill ||
      !!form.nextEpisode !== !!saved.nextEpisode)
  const settingsSummary = saved
    ? [
        saved.limitGB ? t('Homelab.SummaryLimit', { limit: saved.limitGB }) : t('Homelab.SummaryNoLimit'),
        saved.keepDays ? t('Homelab.SummaryDays', { count: saved.keepDays }) : t('Homelab.SummaryForever'),
        saved.backgroundFill && t('Homelab.SummaryFill'),
        saved.nextEpisode && t('Homelab.SummaryNext'),
      ]
        .filter(Boolean)
        .join(' · ')
    : ''

  return (
    <StyledDialog open onClose={handleClose} fullScreen={fullScreen} fullWidth maxWidth='md' ref={ref}>
      <DialogTitle disableTypography>
        <Typography variant='h6'>{t('Homelab.DiskCache')}</Typography>
        {data?.upstream && (
          <Typography variant='caption' color='textSecondary'>
            {t('Homelab.Build', { upstream: data.upstream })}
          </Typography>
        )}
      </DialogTitle>

      <DialogContent dividers>
        {!data ? (
          <Box display='flex' justifyContent='center' p={4}>
            {error ? <Typography color='error'>{error}</Typography> : <CircularProgress color='secondary' />}
          </Box>
        ) : (
          <>
            {!usage.ready ? (
              <Section>
                <Typography color='error'>{t('Homelab.NotReady')}</Typography>
              </Section>
            ) : (
              <Section>
                <UsageRow>
                  <span className='usage-main'>
                    {usage.limit
                      ? t('Homelab.Used', { used: humanizeSize(usage.used) || '0', limit: humanizeSize(usage.limit) })
                      : t('Homelab.UsedNoLimit', { used: humanizeSize(usage.used) || '0' })}
                  </span>
                  {!!usage.diskTotal && (
                    <span className='usage-side'>{t('Homelab.DiskFree', { free: humanizeSize(usage.diskFree) })}</span>
                  )}
                </UsageRow>
                {!!usage.limit && <Bar dark={dark} value={(usage.used / usage.limit) * 100} />}
                {/* the cache path is set but "use disk" is off: the cache is frozen, say so instead of
                    letting the leftovers sit there with the whole homelab UI hidden */}
                {!usage.useDisk && (
                  <Typography variant='body2' color='error' style={{ marginTop: 10 }}>
                    {t('Homelab.DiskOff')}
                  </Typography>
                )}
              </Section>
            )}

            {saved && (
              <Section>
                <SettingsRow dark={dark}>
                  <div className='settings-summary'>
                    {t('Homelab.PersistentShort')}
                    <small>
                      {saved.persistentCache ? settingsSummary : t('Homelab.PersistentOff')} · <NtfySummary />
                    </small>
                  </div>
                  <Button size='small' onClick={() => setEditing(!editing)}>
                    {editing ? t('Homelab.SettingsHide') : t('Homelab.Settings')}
                  </Button>
                  <Switch
                    color='secondary'
                    checked={saved.persistentCache}
                    disabled={!usage.ready || !usage.useDisk || busy} // the server rejects it without UseDisk
                    onChange={e => saveSettings({ ...saved, persistentCache: e.target.checked })}
                  />
                </SettingsRow>

                {editing && form && (
                  <SettingsForm>
                    <Typography variant='subtitle2'>{t('Homelab.PersistentShort')}</Typography>
                    <Typography variant='body2' color='textSecondary'>
                      {t('Homelab.PersistentHelp')}
                    </Typography>
                    <FormControlLabel
                      control={
                        <Switch
                          color='secondary'
                          checked={!!form.backgroundFill}
                          onChange={e => setForm({ ...form, backgroundFill: e.target.checked })}
                        />
                      }
                      label={t('Homelab.BackgroundFill')}
                    />
                    <Typography variant='body2' color='textSecondary'>
                      {t('Homelab.BackgroundFillHelp')}
                    </Typography>
                    <FormControlLabel
                      control={
                        <Switch
                          color='secondary'
                          checked={!!form.nextEpisode}
                          onChange={e => setForm({ ...form, nextEpisode: e.target.checked })}
                        />
                      }
                      label={t('Homelab.NextEpisode')}
                    />
                    <Typography variant='body2' color='textSecondary'>
                      {t('Homelab.NextEpisodeHelp')}
                    </Typography>
                    <div className='settings-fields'>
                      <TextField
                        type='number'
                        label={t('Homelab.Limit')}
                        helperText={t('Homelab.LimitHelp')}
                        value={form.limitGB}
                        inputProps={{ min: 0 }}
                        onChange={e => setForm({ ...form, limitGB: e.target.value })}
                      />
                      <TextField
                        type='number'
                        label={t('Homelab.KeepDays')}
                        helperText={t('Homelab.KeepDaysHelp')}
                        value={form.keepDays}
                        inputProps={{ min: 0 }}
                        onChange={e => setForm({ ...form, keepDays: e.target.value })}
                      />
                      <Box pt={1}>
                        <Button
                          variant='contained'
                          color='secondary'
                          disabled={!dirty || busy}
                          onClick={() => saveSettings({ ...form, persistentCache: saved.persistentCache })}
                        >
                          {t('Homelab.Save')}
                        </Button>
                      </Box>
                    </div>
                    <Box mt={2}>
                      <Typography variant='body2'>{t('Homelab.SettingsNoteTitle')}</Typography>
                      <ul style={{ margin: '4px 0 0', paddingInlineStart: 18, opacity: 0.8, fontSize: 13 }}>
                        {t('Homelab.SettingsNote', { returnObjects: true }).map(line => (
                          <li key={line}>{line}</li>
                        ))}
                      </ul>
                    </Box>
                    <HomelabNtfyForm />
                  </SettingsForm>
                )}
              </Section>
            )}

            {(message || error) && (
              <Section>
                <Typography variant='body2' color={error ? 'error' : 'textSecondary'}>
                  {error || message}
                </Typography>
              </Section>
            )}

            {items.length === 0 ? (
              <Box p={3} textAlign='center'>
                <Typography color='textSecondary'>{t('Homelab.Empty')}</Typography>
              </Box>
            ) : (
              <CardList>
                {items.map(item => (
                  <CacheCard
                    key={item.hash}
                    item={item}
                    download={downloads.find(d => d.hash === item.hash)}
                    confirming={confirmHash === item.hash}
                    busy={busy}
                    dark={dark}
                    onRemove={removeItem}
                    onDownload={it => downloadItem(it, 'start')}
                    onStop={it => downloadItem(it, 'stop')}
                    onOpen={it => setInfoHash(it.hash)}
                  />
                ))}
              </CardList>
            )}
          </>
        )}
      </DialogContent>

      <DialogActions>
        {/* mounted only when there is something to clear: UnsafeButton starts its countdown on mount */}
        {items.length > 0 && (
          <UnsafeButton
            timeout={3}
            startIcon={<DeleteIcon />}
            color='secondary'
            onClick={() => call({ action: 'clear' })}
          >
            {t('Homelab.ClearAll')}
          </UnsafeButton>
        )}
        <Button variant='outlined' color='secondary' onClick={handleClose}>
          {t('Homelab.Close')}
        </Button>
      </DialogActions>
      {infoHash && <HomelabTorrentInfoDialog hash={infoHash} handleClose={() => setInfoHash('')} />}
    </StyledDialog>
  )
}
