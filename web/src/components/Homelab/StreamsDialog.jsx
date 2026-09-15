// homelab: "Watching now" in detail (opens from the card above the torrent list, StreamsSummary). Per client what
// matters first: poster, title and episode, the player's time and what is left; where the data comes from and
// whether it keeps up (the file on disk — it does; from peers — their speed against the bitrate of the file); the
// speed with its graph against the bitrate. Technical details — folded (streams.js, torr/homelab_streams.go).
import {
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  Typography,
  useMediaQuery,
  useTheme,
} from '@material-ui/core'
import { useTranslation } from 'react-i18next'
import { StyledDialog } from 'style/CustomMaterialUiStyles'
import { humanizeSize, humanizeSpeed } from 'utils/Utils'
import useOnStandaloneAppOutsideClick from 'utils/useOnStandaloneAppOutsideClick'

import './i18n'
import { parseTitle } from './parseTitle'
import { clientKey, episodeOf, fmtTime, playerTime, sourceOf, speedHistory, useHomelabStreams } from './streams'
import { StreamCard } from './style'

const clock = unix => new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

// the speed over the last minutes against the bitrate of the file (dashed): above the line — the player keeps up
function SpeedGraph({ points, bitrate, dark }) {
  const { t } = useTranslation()
  if (points.length < 2) return <div className='sc-graph-empty'>{t('Homelab.StreamGraphWait')}</div>
  const w = 300
  const h = 40
  const top = Math.max(...points.map(p => p.speed), (bitrate || 0) * 1.3, 1)
  const x = i => (i / (points.length - 1)) * w
  const y = v => h - 1 - (v / top) * (h - 4)
  const line = points.map((p, i) => `${x(i).toFixed(1)},${y(p.speed).toFixed(1)}`).join(' ')
  const color = dark ? '#dee3e5' : '#3d9c6c'
  const minutes = Math.max(1, Math.round((points[points.length - 1].at - points[0].at) / 60000))
  return (
    <div className='sc-graph'>
      <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio='none' role='img' aria-label={t('Homelab.StreamGraph')}>
        <polygon points={`0,${h} ${line} ${w},${h}`} fill={color} opacity='0.15' />
        <polyline points={line} fill='none' stroke={color} strokeWidth='2' vectorEffect='non-scaling-stroke' />
        {bitrate > 0 && (
          <line
            x1='0'
            x2={w}
            y1={y(bitrate)}
            y2={y(bitrate)}
            stroke={dark ? '#ffb74d' : '#e65100'}
            strokeWidth='1.5'
            strokeDasharray='5 4'
            vectorEffect='non-scaling-stroke'
          />
        )}
      </svg>
      <div className='sc-graph-legend'>
        <span>{t('Homelab.StreamGraphAgo', { count: minutes })}</span>
        {bitrate > 0 && <span className='sc-graph-bitrate'>{t('Homelab.StreamGraphBitrate')}</span>}
        <span>{t('Homelab.StreamGraphNow')}</span>
      </div>
    </div>
  )
}

function ClientCard({ c, dark }) {
  const { t } = useTranslation()
  const { title, subtitle } = parseTitle(c.title || c.path)
  const time = playerTime(c)
  const src = sourceOf(c)
  const watched = Math.max(1, Date.now() / 1000 - c.since)

  let state = t('Homelab.StreamStateActive')
  if (c.ended) state = t('Homelab.StreamEnded')
  else if (!c.active) state = t('Homelab.StreamPaused')

  // whether it keeps up: from disk it does; from peers — their speed against the bitrate of the file
  let health = { tone: 'muted', text: '' }
  if (src.kind === 'disk') health = { tone: 'good', text: t('Homelab.StreamHealthDisk') }
  else if (src.kind === 'partial') {
    const net = c.netSpeed || 0
    const peers = c.totalPeers ? t('Homelab.StreamPeers', { active: c.peers, total: c.totalPeers }) : ''
    const base = t('Homelab.StreamHealthPartial', {
      percent: Math.floor(src.share * 100),
      speed: net > 0 ? humanizeSpeed(net) : t('Homelab.StreamNetIdle'),
    })
    if (time && net >= time.bitrate * 1.1)
      health = { tone: 'good', text: `${base} — ${t('Homelab.StreamHealthEnough')}` }
    else if (time) {
      health = {
        tone: 'warn',
        text: `${base} — ${t('Homelab.StreamHealthSlow', { bitrate: humanizeSpeed(time.bitrate) })}`,
      }
    } else health = { tone: 'muted', text: base }
    if (peers) health.text = `${health.text} · ${peers}`
  }

  const details = [
    [t('Homelab.StreamFactFile'), (c.path || '').split('/').pop()],
    [t('Homelab.StreamFactSince'), `${clock(c.since)} · ${fmtTime(watched)}`],
    [
      t('Homelab.StreamFactSent'),
      `${humanizeSize(c.bytes) || '0'} · ${t('Homelab.StreamAverage', {
        speed: humanizeSpeed(c.bytes / watched) || '0',
      })}`,
    ],
    [t('Homelab.StreamFactPosition'), `${humanizeSize(c.offset) || '0'} / ${humanizeSize(c.fileLength) || '?'}`],
    [t('Homelab.StreamFactConnections'), String(c.connections || 0)],
    [t('Homelab.StreamFactAddress'), c.ip],
    [t('Homelab.StreamFactPlayer'), c.ua || '—'],
  ]

  return (
    <StreamCard dark={dark} className={c.ended ? 'sc-ended' : ''}>
      <div className='sc-top'>
        <div className='sc-poster'>{c.poster ? <img src={c.poster} alt='' loading='lazy' /> : '▶'}</div>
        <div className='sc-what'>
          <div className='sc-title' title={c.title}>
            {title}
          </div>
          {(episodeOf(c.path) || subtitle) && <div className='sc-sub'>{episodeOf(c.path) || subtitle}</div>}
          <div className='sc-state'>
            <span className={c.active ? 'sc-dot sc-dot-live' : 'sc-dot'} />
            {state} · {c.device}
          </div>
        </div>
        <div className='sc-speed'>
          <b>{c.active ? humanizeSpeed(c.speed) : '—'}</b>
          {time && <span>{t('Homelab.StreamBitrate', { bitrate: humanizeSpeed(time.bitrate) })}</span>}
        </div>
      </div>

      <div className='sc-time'>
        {time ? (
          <>
            <b>{fmtTime(time.at)}</b>
            <span>/ {fmtTime(time.total)}</span>
            <span className='sc-left'>{t('Homelab.StreamLeft', { time: fmtTime(time.left) })}</span>
          </>
        ) : (
          <b>{Math.round(c.position * 100)}%</b>
        )}
      </div>
      <div className='sc-bar'>
        <div style={{ width: `${Math.max(0.5, c.position * 100)}%` }} />
      </div>

      {health.text && <div className={`sc-health sc-${health.tone}`}>{health.text}</div>}

      <SpeedGraph points={speedHistory(c)} bitrate={time?.bitrate} dark={dark} />

      <details className='sc-details'>
        <summary>{t('Homelab.StreamDetails')}</summary>
        <dl>
          {details.map(([name, value]) => (
            <div key={name}>
              <dt>{name}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </details>
    </StreamCard>
  )
}

export default function HomelabStreamsDialog({ handleClose }) {
  const { t } = useTranslation()
  const fullScreen = useMediaQuery('@media (max-width:930px)')
  const dark = useTheme().palette.type === 'dark'
  const ref = useOnStandaloneAppOutsideClick(handleClose)
  const clients = useHomelabStreams() || []
  const watching = clients.filter(c => !c.ended)
  const total = watching.reduce((sum, c) => sum + c.speed, 0)

  return (
    <StyledDialog open onClose={handleClose} fullScreen={fullScreen} fullWidth maxWidth='sm' ref={ref}>
      <DialogTitle disableTypography>
        <Typography variant='h6'>{t('Homelab.StreamsTitleDialog')}</Typography>
        {watching.length > 1 && (
          <Typography variant='caption' color='textSecondary'>
            {t('Homelab.StreamsTotal', { count: watching.length, speed: humanizeSpeed(total) || '0' })}
          </Typography>
        )}
      </DialogTitle>
      <DialogContent dividers>
        {clients.length === 0 ? (
          <Typography color='textSecondary'>{t('Homelab.StreamsNobody')}</Typography>
        ) : (
          clients.map(c => <ClientCard key={clientKey(c)} c={c} dark={dark} />)
        )}
        <Typography variant='caption' color='textSecondary' component='div' style={{ marginTop: 12 }}>
          {t('Homelab.StreamsNote')}
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button variant='outlined' color='secondary' onClick={handleClose}>
          {t('Homelab.Close')}
        </Button>
      </DialogActions>
    </StyledDialog>
  )
}
