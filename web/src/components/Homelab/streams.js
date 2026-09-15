// homelab: who is watching now (server: torr/homelab_streams.go, GET /homelab/streams) — one shared poller for the
// summary above the torrent list, the badges on the cards and the torrent details.
import axios from 'axios'
import ptt from 'parse-torrent-title'
import { useEffect, useState } from 'react'
import { getTorrServerHost } from 'utils/Hosts'

const REFRESH_MS = 3000
const HISTORY = 60 // points of speed history per client: 3 minutes of polls

let snapshot = null
let timer = null
const listeners = new Set()
const history = new Map() // client key → [{at, speed}], kept while the page is open

export const clientKey = c => `${c.ip}|${c.ua}|${c.hash}|${c.path}`

const load = () =>
  axios
    .get(`${getTorrServerHost()}/homelab/streams`)
    .then(({ data }) => {
      snapshot = data.clients || []
      const now = Date.now()
      const seen = new Set()
      snapshot.forEach(c => {
        const key = clientKey(c)
        seen.add(key)
        const points = history.get(key) || []
        points.push({ at: now, speed: c.ended ? 0 : c.speed })
        history.set(key, points.slice(-HISTORY))
      })
      history.forEach((_, key) => !seen.has(key) && history.delete(key))
      listeners.forEach(listener => listener(snapshot))
    })
    .catch(() => {}) // an old server: nothing shown

// speed of a client over the last minutes (polled while the page was open)
export const speedHistory = c => history.get(clientKey(c)) || []

// the clients: [{device, ip, ua, hash, title, path, position, speed, bytes, since, active, connections, ended}]
export function useHomelabStreams() {
  const [clients, setClients] = useState(snapshot)

  useEffect(() => {
    listeners.add(setClients)
    if (!timer) {
      load()
      timer = setInterval(load, REFRESH_MS)
    } else if (snapshot) {
      setClients(snapshot)
    }
    return () => {
      listeners.delete(setClients)
      if (!listeners.size) {
        clearInterval(timer)
        timer = null
      }
    }
  }, [])

  return clients
}

export const useTorrentStreams = hash => (useHomelabStreams() || []).filter(c => c.hash === hash)

// "S1E3" for an episode, nothing for a film
export function episodeOf(filePath) {
  const { season, episode } = ptt.parse((filePath || '').split('/').pop())
  if (!episode) return ''
  return season ? `S${season}E${episode}` : `E${episode}`
}

// 1:02:15 / 42:07
export function fmtTime(seconds) {
  const s = Math.max(0, Math.round(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`
}

// the time of the player, if the duration of the file is known (≈ by the byte position)
export function playerTime(c) {
  if (!c.duration) return null
  const at = c.position * c.duration
  return { at, total: c.duration, left: c.duration - at, bitrate: c.fileLength / c.duration } // bitrate: bytes/s
}

// where the data comes from: the whole file on disk, a part, or unknown
export function sourceOf(c) {
  if (c.onDisk < 0) return { kind: 'unknown' }
  if (c.onDisk >= 0.999) return { kind: 'disk' }
  return { kind: 'partial', share: c.onDisk }
}
