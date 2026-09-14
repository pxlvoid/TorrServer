// homelab: what the persistent disk cache holds — usage, settings, per-torrent pin / remove.
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
import MovieIcon from '@material-ui/icons/Movie'
import StarIcon from '@material-ui/icons/Star'
import StarBorderIcon from '@material-ui/icons/StarBorder'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { StyledDialog } from 'style/CustomMaterialUiStyles'
import { getTorrServerHost } from 'utils/Hosts'
import { humanizeSize } from 'utils/Utils'
import useOnStandaloneAppOutsideClick from 'utils/useOnStandaloneAppOutsideClick'

import UnsafeButton from '../UnsafeButton'
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

function CacheCard({ item, busy, dark, onPin, onRemove }) {
  const { t } = useTranslation()
  const relativeTime = useRelativeTime()
  const [posterFailed, setPosterFailed] = useState(false)
  const fullName = item.title || item.name || item.hash
  const { title, subtitle } = parseTitle(fullName)
  const percent = item.totalLength ? (item.size / item.totalLength) * 100 : null
  const stats = [
    item.totalLength
      ? t('Homelab.SizeOf', { size: humanizeSize(item.size), total: humanizeSize(item.totalLength) })
      : humanizeSize(item.size),
    relativeTime(item.lastAccess),
  ].filter(Boolean)

  return (
    <Card pinned={item.pinned} dark={dark}>
      <Poster>
        {item.poster && !posterFailed ? (
          <img src={item.poster} alt='' loading='lazy' onError={() => setPosterFailed(true)} />
        ) : (
          <MovieIcon />
        )}
        {item.playing && (
          <PlayingBadge dark={dark} title={t('Homelab.Playing')}>
            {t('Homelab.PlayingShort')}
          </PlayingBadge>
        )}
      </Poster>

      <Info title={fullName}>
        <div className='card-title'>{title}</div>
        {subtitle && <div className='card-subtitle'>{subtitle}</div>}
        <div className='card-stats'>
          {percent !== null && <Bar thin dark={dark} value={percent} style={{ marginBottom: 6 }} />}
          {stats.join(' · ')}
        </div>
      </Info>

      <Actions>
        <Tooltip title={item.pinned ? t('Homelab.Unpin') : t('Homelab.Pin')}>
          <span>
            <IconButton disabled={busy} onClick={() => onPin(item)} color={item.pinned ? 'secondary' : 'default'}>
              {item.pinned ? <StarIcon /> : <StarBorderIcon />}
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title={item.playing ? t('Homelab.RemovePlaying') : t('Homelab.Remove')}>
          <span>
            <IconButton disabled={busy} onClick={() => onRemove(item)}>
              <DeleteIcon />
            </IconButton>
          </span>
        </Tooltip>
      </Actions>
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

  const saveSettings = sets => {
    setBusy(true)
    axios
      .post(settingsHost(), {
        persistentCache: sets.persistentCache,
        limitGB: Math.max(0, parseInt(sets.limitGB, 10) || 0),
        keepDays: Math.max(0, parseInt(sets.keepDays, 10) || 0),
        backgroundFill: !!sets.backgroundFill, // the API stores the whole struct: never omit a field
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
  const items = data?.items || []
  const dirty =
    form &&
    saved &&
    (String(form.limitGB) !== String(saved.limitGB) ||
      String(form.keepDays) !== String(saved.keepDays) ||
      !!form.backgroundFill !== !!saved.backgroundFill)
  const settingsSummary = saved
    ? [
        saved.limitGB ? t('Homelab.SummaryLimit', { limit: saved.limitGB }) : t('Homelab.SummaryNoLimit'),
        saved.keepDays ? t('Homelab.SummaryDays', { count: saved.keepDays }) : t('Homelab.SummaryForever'),
        saved.backgroundFill && t('Homelab.SummaryFill'),
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
              </Section>
            )}

            {saved && (
              <Section>
                <SettingsRow dark={dark}>
                  <div className='settings-summary'>
                    {t('Homelab.PersistentShort')}
                    <small>{saved.persistentCache ? settingsSummary : t('Homelab.PersistentOff')}</small>
                  </div>
                  <Button size='small' disabled={!usage.ready} onClick={() => setEditing(!editing)}>
                    {editing ? t('Homelab.Cancel') : t('Homelab.Edit')}
                  </Button>
                  <Switch
                    color='secondary'
                    checked={saved.persistentCache}
                    disabled={!usage.ready || busy}
                    onChange={e => saveSettings({ ...saved, persistentCache: e.target.checked })}
                  />
                </SettingsRow>

                {editing && form && (
                  <SettingsForm>
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
                    busy={busy}
                    dark={dark}
                    onPin={it => call({ action: 'pin', hash: it.hash, pinned: !it.pinned })}
                    onRemove={it => call({ action: 'remove', hash: it.hash })}
                  />
                ))}
              </CardList>
            )}
          </>
        )}
      </DialogContent>

      <DialogActions>
        {/* mounted only when there is something to clear: UnsafeButton starts its countdown on mount */}
        {items.some(it => !it.pinned) && (
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
    </StyledDialog>
  )
}
