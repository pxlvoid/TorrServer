// homelab: "Watching now" in detail (opens from the card above the torrent list, StreamsSummary): per client the
// player and its address, what exactly (title, episode, file), where the player is, speed now / on average and its
// graph over the last minutes, how much was sent, since when, and where the data comes from — the share of the file
// on disk and what the torrent downloads from peers now (streams.js, torr/homelab_streams.go).
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
import { clientKey, episodeOf, speedHistory, useHomelabStreams } from './streams'
import { Bar, StreamCard } from './style'

const clock = unix => new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

function useDuration() {
  const { t } = useTranslation()
  return seconds => {
    const minutes = Math.max(1, Math.round(seconds / 60))
    return minutes < 60
      ? t('Homelab.Minutes', { count: minutes })
      : t('Homelab.HoursMinutes', { h: Math.floor(minutes / 60), m: minutes % 60 })
  }
}

// speed over the last minutes: an area under a line, the highest point labelled
function Sparkline({ points, dark }) {
  const { t } = useTranslation()
  if (points.length < 2) return <div className='stream-spark-empty'>{t('Homelab.StreamGraphWait')}</div>
  const w = 300
  const h = 48
  const max = Math.max(...points.map(p => p.speed), 1)
  const x = i => (i / (points.length - 1)) * w
  const y = v => h - 2 - (v / max) * (h - 6)
  const line = points.map((p, i) => `${x(i).toFixed(1)},${y(p.speed).toFixed(1)}`).join(' ')
  const color = dark ? '#dee3e5' : '#3d9c6c'
  return (
    <div className='stream-spark'>
      <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio='none' role='img' aria-label={t('Homelab.StreamGraph')}>
        <polygon points={`0,${h} ${line} ${w},${h}`} fill={color} opacity='0.18' />
        <polyline points={line} fill='none' stroke={color} strokeWidth='2' vectorEffect='non-scaling-stroke' />
      </svg>
      <div className='stream-spark-legend'>
        <span>{t('Homelab.StreamGraph')}</span>
        <span>{t('Homelab.StreamPeak', { speed: humanizeSpeed(max) || '0' })}</span>
      </div>
    </div>
  )
}

function ClientCard({ c, dark }) {
  const { t } = useTranslation()
  const duration = useDuration()
  const { title, subtitle } = parseTitle(c.title || c.path)
  const file = (c.path || '').split('/').pop()
  const seconds = Math.max(1, Date.now() / 1000 - c.since)
  let state = t('Homelab.StreamStateActive')
  if (c.ended) state = t('Homelab.StreamEnded')
  else if (!c.active) state = t('Homelab.StreamPaused')

  let net = t('Homelab.StreamNetIdle')
  if (c.netSpeed > 0) net = humanizeSpeed(c.netSpeed)
  const peers = c.totalPeers ? t('Homelab.StreamPeers', { active: c.peers, total: c.totalPeers }) : ''

  const facts = [
    [t('Homelab.StreamFactSince'), `${clock(c.since)} (${duration(seconds)})`],
    [
      t('Homelab.StreamFactSent'),
      `${humanizeSize(c.bytes) || '0'} · ${t('Homelab.StreamAverage', {
        speed: humanizeSpeed(c.bytes / seconds) || '0',
      })}`,
    ],
    [
      t('Homelab.StreamFactDisk'),
      c.onDisk >= 0
        ? `${Math.floor(c.onDisk * 100)}%${c.onDisk >= 0.999 ? ` — ${t('Homelab.StreamFromDisk')}` : ''}`
        : '—',
    ],
    [t('Homelab.StreamFactNet'), [net, peers].filter(Boolean).join(' · ')],
    [t('Homelab.StreamFactConnections'), String(c.connections || 0)],
    [t('Homelab.StreamFactAddress'), c.ip],
  ]

  return (
    <StreamCard dark={dark} className={c.ended ? 'stream-card-ended' : ''}>
      <div className='stream-card-head'>
        <div>
          <span className='stream-card-device'>{c.device}</span>
          <span className={`stream-card-state ${c.active ? 'stream-card-live' : ''}`}>{state}</span>
        </div>
        <span className='stream-card-speed'>{c.active ? humanizeSpeed(c.speed) : '—'}</span>
      </div>

      <div className='stream-card-title' title={c.title}>
        {title}
        {(episodeOf(c.path) || subtitle) && <span className='stream-card-sub'>{episodeOf(c.path) || subtitle}</span>}
      </div>
      <div className='stream-card-file' title={c.path}>
        {file}
      </div>

      <Bar thin dark={dark} value={c.position * 100} />
      <div className='stream-card-pos'>
        {t('Homelab.StreamPosition', {
          percent: Math.round(c.position * 100),
          at: humanizeSize(c.offset) || '0',
          total: humanizeSize(c.fileLength) || '?',
        })}
      </div>

      <Sparkline points={speedHistory(c)} dark={dark} />

      <dl className='stream-card-facts'>
        {facts.map(([name, value]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <div className='stream-card-ua'>
        {t('Homelab.StreamFactPlayer')}: {c.ua || '—'}
      </div>
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
    <StyledDialog open onClose={handleClose} fullScreen={fullScreen} fullWidth maxWidth='md' ref={ref}>
      <DialogTitle disableTypography>
        <Typography variant='h6'>{t('Homelab.StreamsWatching', { count: watching.length })}</Typography>
        <Typography variant='caption' color='textSecondary'>
          {[total > 0 && t('Homelab.StreamsTotal', { speed: humanizeSpeed(total) }), t('Homelab.StreamsNote')]
            .filter(Boolean)
            .join(' · ')}
        </Typography>
      </DialogTitle>
      <DialogContent dividers>
        {clients.length === 0 ? (
          <Typography color='textSecondary'>{t('Homelab.StreamsNobody')}</Typography>
        ) : (
          clients.map(c => <ClientCard key={clientKey(c)} c={c} dark={dark} />)
        )}
      </DialogContent>
      <DialogActions>
        <Button variant='outlined' color='secondary' onClick={handleClose}>
          {t('Homelab.Close')}
        </Button>
      </DialogActions>
    </StyledDialog>
  )
}
