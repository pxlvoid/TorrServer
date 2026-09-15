// homelab: ntfy notifications in the disk cache dialog (server: torrstor/homelab_ntfy.go, POST /homelab/ntfy).
// A row with the summary; "Edit" opens the form: server, topic (the channel), token, events; "Check" sends a test
// notification with the form as it is, without saving. The stored token never comes back to the browser.
import { Box, Button, Checkbox, FormControlLabel, TextField, Typography } from '@material-ui/core'
import axios from 'axios'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getTorrServerHost } from 'utils/Hosts'

import './i18n'
import { SettingsForm, SettingsRow } from './style'

const ntfyHost = () => `${getTorrServerHost()}/homelab/ntfy`
const EVENTS = ['download_done', 'download_error', 'disk', 'started']

const errorText = err => err?.response?.data?.error || err?.message || String(err)

export default function HomelabNtfySettings({ dark }) {
  const { t } = useTranslation()
  const [saved, setSaved] = useState(null)
  const [form, setForm] = useState(null)
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState({ text: '', error: false })

  useEffect(() => {
    axios
      .post(ntfyHost(), { action: 'get' })
      .then(({ data }) => setSaved(data))
      .catch(() => {}) // an old server: no row
  }, [])

  if (!saved) return null

  const open = () => {
    setForm({ url: saved.url, topic: saved.topic, token: '', tokenClear: false, events: { ...saved.events } })
    setStatus({ text: '', error: false })
    setEditing(true)
  }

  const send = action => {
    setBusy(true)
    return axios
      .post(ntfyHost(), { action, ...form })
      .then(({ data }) => {
        if (action === 'save') {
          setSaved(data)
          setEditing(false)
        }
        setStatus({ text: action === 'test' ? t('Homelab.NtfyTestOk') : t('Homelab.Saved'), error: false })
      })
      .catch(err => setStatus({ text: errorText(err), error: true }))
      .finally(() => setBusy(false))
  }

  const eventsOn = EVENTS.filter(ev => saved.events?.[ev]).length
  const summary = saved.url
    ? t('Homelab.NtfyOn', { target: `${saved.url.replace(/^https?:\/\//, '')}/${saved.topic}`, count: eventsOn })
    : t('Homelab.NtfyOff')

  return (
    <>
      <SettingsRow dark={dark}>
        <div className='settings-summary'>
          {t('Homelab.NtfyTitle')}
          <small>{summary}</small>
        </div>
        <Button size='small' onClick={() => (editing ? setEditing(false) : open())}>
          {editing ? t('Homelab.Cancel') : t('Homelab.Edit')}
        </Button>
      </SettingsRow>

      {editing && form && (
        <SettingsForm>
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

          <Box mt={2}>
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

          <Box mt={2} display='flex' alignItems='center' flexWrap='wrap' style={{ gap: 12 }}>
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
        </SettingsForm>
      )}
      {!editing && status.text && (
        <Typography variant='body2' color={status.error ? 'error' : 'textSecondary'} style={{ marginTop: 8 }}>
          {status.text}
        </Typography>
      )}
    </>
  )
}
