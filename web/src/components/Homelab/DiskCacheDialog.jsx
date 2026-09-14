// homelab: what the persistent disk cache holds — usage, settings, per-torrent pin / remove.
// Accent is "secondary": in the dark theme TorrServer's primary (#323637) is the dialog background.
import axios from 'axios'
import {
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  LinearProgress,
  List,
  ListItem,
  ListItemAvatar,
  ListItemSecondaryAction,
  ListItemText,
  Switch,
  TextField,
  Tooltip,
  Typography,
  useMediaQuery,
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

function CacheItem({ item, busy, onPin, onRemove }) {
  const { t } = useTranslation()
  const relativeTime = useRelativeTime()
  const title = item.title || item.name || item.hash.slice(0, 12)
  const percent = item.totalLength ? Math.round((item.size / item.totalLength) * 100) : null
  const details = [
    humanizeSize(item.size),
    percent !== null && t('Homelab.OfTorrent', { percent }),
    relativeTime(item.lastAccess),
  ].filter(Boolean)

  return (
    // room for two icon buttons of ListItemSecondaryAction (MUI reserves only one)
    <ListItem divider style={{ paddingRight: 112 }}>
      <ListItemAvatar>
        <Avatar variant='rounded' src={item.poster || undefined} alt=''>
          <MovieIcon />
        </Avatar>
      </ListItemAvatar>
      <ListItemText
        primary={
          <Box display='flex' alignItems='center' flexWrap='wrap' style={{ gap: 6 }}>
            <span style={{ wordBreak: 'break-word' }}>{title}</span>
            {item.playing ? (
              <Chip size='small' color='secondary' label={t('Homelab.Playing')} />
            ) : item.open ? (
              <Chip size='small' variant='outlined' label={t('Homelab.Open')} />
            ) : null}
          </Box>
        }
        secondary={details.join(' · ')}
      />
      <ListItemSecondaryAction>
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
      </ListItemSecondaryAction>
    </ListItem>
  )
}

export default function DiskCacheDialog({ handleClose }) {
  const { t } = useTranslation()
  const fullScreen = useMediaQuery('@media (max-width:930px)')
  const ref = useOnStandaloneAppOutsideClick(handleClose)

  const [data, setData] = useState(null)
  const [form, setForm] = useState(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const apply = useCallback(
    resp => {
      setData(resp)
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

  const saveSettings = () => {
    setBusy(true)
    const sets = {
      persistentCache: form.persistentCache,
      limitGB: Math.max(0, parseInt(form.limitGB, 10) || 0),
      keepDays: Math.max(0, parseInt(form.keepDays, 10) || 0),
    }
    axios
      .post(settingsHost(), sets)
      .then(({ data }) => {
        setForm(data)
        setMessage(t('Homelab.Saved'))
        setError('')
        return call({ action: 'list' }, false)
      })
      .catch(err => setError(errorText(err)))
      .finally(() => setBusy(false))
  }

  const usage = data?.usage
  const items = data?.items || []
  const dirty =
    form &&
    data &&
    (form.persistentCache !== data.settings.persistentCache ||
      String(form.limitGB) !== String(data.settings.limitGB) ||
      String(form.keepDays) !== String(data.settings.keepDays))
  const usedPercent = usage?.limit ? Math.min(100, (usage.used / usage.limit) * 100) : 0

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
            {error ? <Typography color='error'>{error}</Typography> : <CircularProgress />}
          </Box>
        ) : (
          <>
            {!usage.ready && (
              <Box mb={2}>
                <Typography color='error'>{t('Homelab.NotReady')}</Typography>
              </Box>
            )}

            {usage.ready && (
              <Box mb={2}>
                <Typography>
                  {usage.limit
                    ? t('Homelab.Used', { used: humanizeSize(usage.used) || '0', limit: humanizeSize(usage.limit) })
                    : t('Homelab.UsedNoLimit', { used: humanizeSize(usage.used) || '0' })}
                </Typography>
                {!!usage.limit && (
                  <Box my={1}>
                    <LinearProgress variant='determinate' color='secondary' value={usedPercent} />
                  </Box>
                )}
                {!!usage.diskTotal && (
                  <Typography variant='caption' color='textSecondary'>
                    {t('Homelab.DiskFree', { free: humanizeSize(usage.diskFree), path: usage.path })}
                  </Typography>
                )}
              </Box>
            )}

            {form && (
              <Box mb={2}>
                <FormControlLabel
                  control={
                    <Switch
                      color='secondary'
                      checked={form.persistentCache}
                      disabled={!usage.ready}
                      onChange={e => setForm({ ...form, persistentCache: e.target.checked })}
                    />
                  }
                  label={t('Homelab.Persistent')}
                />
                <Typography variant='body2' color='textSecondary'>
                  {t('Homelab.PersistentHelp')}
                </Typography>
                <Box display='flex' flexWrap='wrap' alignItems='flex-start' mt={1} style={{ gap: 16 }}>
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
                    <Button variant='contained' color='secondary' disabled={!dirty || busy} onClick={saveSettings}>
                      {t('Homelab.Save')}
                    </Button>
                  </Box>
                </Box>
              </Box>
            )}

            {(message || error) && (
              <Box mb={1}>
                <Typography variant='body2' color={error ? 'error' : 'textSecondary'}>
                  {error || message}
                </Typography>
              </Box>
            )}

            {items.length === 0 ? (
              <Box p={3} textAlign='center'>
                <Typography color='textSecondary'>{t('Homelab.Empty')}</Typography>
              </Box>
            ) : (
              <List disablePadding>
                {items.map(item => (
                  <CacheItem
                    key={item.hash}
                    item={item}
                    busy={busy}
                    onPin={it => call({ action: 'pin', hash: it.hash, pinned: !it.pinned })}
                    onRemove={it => call({ action: 'remove', hash: it.hash })}
                  />
                ))}
              </List>
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
