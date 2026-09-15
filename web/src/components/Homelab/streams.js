// homelab: who is watching now (server: torr/homelab_streams.go, GET /homelab/streams) — one shared poller for the
// summary above the torrent list, the badges on the cards and the torrent details.
import axios from 'axios'
import ptt from 'parse-torrent-title'
import { useEffect, useState } from 'react'
import { getTorrServerHost } from 'utils/Hosts'

const REFRESH_MS = 3000

let snapshot = null
let timer = null
const listeners = new Set()

const load = () =>
  axios
    .get(`${getTorrServerHost()}/homelab/streams`)
    .then(({ data }) => {
      snapshot = data.clients || []
      listeners.forEach(listener => listener(snapshot))
    })
    .catch(() => {}) // an old server: nothing shown

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
