// homelab: "download to disk" — the whole torrent or chosen files into the persistent cache
// (server: torr/homelab_download.go). One shared poller of /homelab/download per torrent, however many
// buttons show it (every file of a season has one).
import axios from 'axios'
import { useEffect, useState } from 'react'
import { getTorrServerHost } from 'utils/Hosts'
import { humanizeSize } from 'utils/Utils'

import { refreshHomelabCache } from './store'

const REFRESH_MS = 3000

export const homelabDownloadHost = () => `${getTorrServerHost()}/homelab/download`

const pollers = new Map() // hash → { data, listeners, timer }

const publish = (hash, patch) => {
  const poller = pollers.get(hash)
  if (!poller) return
  poller.data = { ...poller.data, ...patch }
  poller.listeners.forEach(listener => listener(poller.data))
}

const fetchStatus = hash =>
  axios
    .post(homelabDownloadHost(), { action: 'status', hash })
    .then(({ data }) => publish(hash, data))
    .catch(() => {}) // old server or no access: download buttons stay hidden

// start / stop; a refused start ({error: code, need, avail}) is kept as startError until the next action
export function homelabDownloadAction(hash, action, files) {
  publish(hash, { startError: null })
  return axios
    .post(homelabDownloadHost(), { action, hash, files })
    .then(({ data }) => {
      publish(hash, data)
      refreshHomelabCache() // badges, summary and the disk cache dialog
      return data
    })
    .catch(err => {
      const data = err?.response?.data
      const startError = data?.error ? data : { error: 'failed' }
      publish(hash, { startError })
      return { startError }
    })
}

// { enabled, job, files: [{id, length, done}], startError } of a torrent, refreshed every few seconds
export function useHomelabDownload(hash) {
  const [data, setData] = useState(() => pollers.get(hash)?.data || null)

  useEffect(() => {
    if (!hash) return undefined
    let poller = pollers.get(hash)
    if (!poller) {
      poller = { data: null, listeners: new Set(), timer: null }
      pollers.set(hash, poller)
    }
    poller.listeners.add(setData)
    if (!poller.timer) {
      fetchStatus(hash)
      poller.timer = setInterval(() => fetchStatus(hash), REFRESH_MS)
    } else if (poller.data) {
      setData(poller.data)
    }
    return () => {
      poller.listeners.delete(setData)
      if (!poller.listeners.size) {
        clearInterval(poller.timer)
        pollers.delete(hash)
      }
    }
  }, [hash])

  return data
}

// the job covers this file (a job without files is the whole torrent)
export const jobHasFile = (job, id) => !!job && (!job.files?.length || job.files.includes(id))

// why a job can not run — codes of the server (HomelabDl* in torr/homelab_download.go)
export const downloadErrorText = (t, err) =>
  err?.error
    ? t(`Homelab.DownloadError.${err.error}`, {
        need: humanizeSize(err.need) || '0',
        avail: humanizeSize(err.avail) || '0',
        defaultValue: t('Homelab.DownloadError.failed'),
      })
    : ''
