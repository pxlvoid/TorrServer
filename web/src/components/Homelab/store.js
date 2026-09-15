// homelab: one shared poller of /homelab/cache for the main screen (summary, badges on every card,
// the disk timeline in torrent details) — a single request per interval however many cards there are.
import axios from 'axios'
import { useEffect, useState } from 'react'
import { getTorrServerHost } from 'utils/Hosts'

const REFRESH_MS = 10000

let snapshot = null
let timer = null
const listeners = new Set()

export const homelabCacheHost = () => `${getTorrServerHost()}/homelab/cache`

const publish = data => {
  snapshot = data
  listeners.forEach(listener => listener(data))
}

export function refreshHomelabCache() {
  return axios
    .post(homelabCacheHost(), { action: 'list' })
    .then(({ data }) => publish(data))
    .catch(() => {}) // old server or no access: the homelab UI just stays hidden
}

// Lets the disk cache dialog push fresh data after remove / pin / clear without waiting for the poller.
export const publishHomelabCache = publish

export function useHomelabCache() {
  const [data, setData] = useState(snapshot)

  useEffect(() => {
    listeners.add(setData)
    if (!timer) {
      refreshHomelabCache()
      timer = setInterval(refreshHomelabCache, REFRESH_MS)
    } else if (snapshot) {
      setData(snapshot)
    }
    return () => {
      listeners.delete(setData)
      if (!listeners.size) {
        clearInterval(timer)
        timer = null
      }
    }
  }, [])

  return data
}
