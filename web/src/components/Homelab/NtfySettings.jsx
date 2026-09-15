// homelab: ntfy notifications in the disk cache dialog (server: torrstor/homelab_ntfy.go, POST /homelab/ntfy).
// A short summary for the settings row of the dialog (NtfySummary) and the form in its settings (default export):
// server, topic (the channel), token, events; "Check" sends a test notification with the form as it is, without
// saving. The stored token never comes back to the browser. Both share the settings (a save updates the summary).
import { Box, Button, Checkbox, FormControlLabel, TextField, Typography } from '@material-ui/core'
import axios from 'axios'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getTorrServerHost } from 'utils/Hosts'

import './i18n'

const ntfyHost = () => `${getTorrServerHost()}/homelab/ntfy`
const EVENTS = ['download_done', 'download_error', 'disk', 'started']

const errorText = err => err?.response?.data?.error || err?.message || String(err)

let snapshot = null
const listeners = new Set()
const publish = data => {
  snapshot = data
  listeners.forEach(listener => listener(data))
}

function useNtfy() {
  const [saved, setSaved] = useState(snapshot)
  useEffect(() => {
    listeners.add(setSaved)
    axios
      .post(ntfyHost(), { action: 'get' })
      .then(({ data }) => publish(data))
      .catch(() => {}) // an old server: nothing shown
    return () => listeners.delete(setSaved)
  }, [])
  return saved
}

// "ntfy.example.org/ts" or "notifications off"
export function NtfySummary() {
  const { t } = useTranslation()
  const saved = useNtfy()
  if (!saved) return null
  return saved.url
    ? t('Homelab.NtfyShort', { target: `${saved.url.replace(/^https?:\/\//, '')}/${saved.topic}` })
    : t('Homelab.NtfyShortOff')
}

export default function HomelabNtfyForm() {
  const { t } = useTranslation()
  const saved = useNtfy()
  const [form, setForm] = useState(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState({ text: '', error: false })

  useEffect(() => {
    if (saved && !form) {
      setForm({ url: saved.url, topic: saved.topic, token: '', tokenClear: false, events: { ...saved.events } })
    }
  }, [saved, form])

  if (!saved || !form) return null

  const send = action => {
    setBusy(true)
    return axios
      .post(ntfyHost(), { action, ...form })
      .then(({ data }) => {
        if (action === 'save') {
          publish(data)
          setForm({ ...form, token: '', tokenClear: false })
        }
        setStatus({ text: action === 'test' ? t('Homelab.NtfyTestOk') : t('Homelab.Saved'), error: false })
      })
      .catch(err => setStatus({ text: errorText(err), error: true }))
      .finally(() => setBusy(false))
  }

  return (
    <Box mt={3}>
      <Typography variant='subtitle2'>{t('Homelab.NtfyTitle')}</Typography>
      <div className='settings-fields'>
        <TextField
          label={t('Homelab.NtfyUrl')}
          helperText={t('Homelab.NtfyUrlHelp')}
          placeholder='https://ntfy.example.org'
          value={form.url}
          onChange={e => setForm({ ...form, url: e.target.value })}
        />
        <TextField
          label={t('Homelab.NtfyTopic')}
          helperText={t('Homelab.NtfyTopicHelp')}
          value={form.topic}
          onChange={e => setForm({ ...form, topic: e.target.value })}
        />
        <TextField
          type='password'
          autoComplete='new-password'
          label={t('Homelab.NtfyToken')}
          helperText={saved.tokenSet && !form.tokenClear ? t('Homelab.NtfyTokenSaved') : t('Homelab.NtfyTokenHelp')}
          value={form.token}
          onChange={e => setForm({ ...form, token: e.target.value, tokenClear: false })}
        />
      </div>
      {saved.tokenSet && !form.tokenClear && (
        <Button size='small' onClick={() => setForm({ ...form, token: '', tokenClear: true })}>
          {t('Homelab.NtfyTokenForget')}
        </Button>
      )}

      <Box mt={1}>
        <Typography variant='body2'>{t('Homelab.NtfyEventsTitle')}</Typography>
        {EVENTS.map(ev => (
          <div key={ev}>
            <FormControlLabel
              control={
                <Checkbox
                  size='small'
                  checked={!!form.events[ev]}
                  onChange={e => setForm({ ...form, events: { ...form.events, [ev]: e.target.checked } })}
                />
              }
              label={<Typography variant='body2'>{t(`Homelab.NtfyEvent.${ev}`)}</Typography>}
            />
          </div>
        ))}
      </Box>

      <Box mt={1} display='flex' alignItems='center' flexWrap='wrap' style={{ gap: 12 }}>
        <Button variant='outlined' disabled={busy || !form.url} onClick={() => send('test')}>
          {t('Homelab.NtfyTest')}
        </Button>
        <Button variant='contained' color='secondary' disabled={busy} onClick={() => send('save')}>
          {t('Homelab.Save')}
        </Button>
        {status.text && (
          <Typography variant='body2' color={status.error ? 'error' : 'textSecondary'}>
            {status.text}
          </Typography>
        )}
      </Box>
    </Box>
  )
}
