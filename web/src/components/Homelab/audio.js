// homelab: the audio track served to players (server: torr/homelab_audio.go) — one shared poller of
// /homelab/audio per torrent for the choice under "Info" and the "Audio" column of the file table, so a change in
// one of them shows in the other at once. The starts of some episodes may not be downloaded yet: while a file
// is not ready its tracks are asked again for a while.
import axios from 'axios'
import { useEffect, useState } from 'react'
import { getTorrServerHost } from 'utils/Hosts'

const RETRY_MS = 5000
const RETRIES = 24 // the starts of the episodes arrive within a couple of minutes, or not at all

export const homelabAudioHost = () => `${getTorrServerHost()}/homelab/audio`

const pollers = new Map() // hash → { data, listeners, timer, tries }

const publish = (hash, data) => {
  const poller = pollers.get(hash)
  if (!poller) return
  poller.data = data
  poller.listeners.forEach(listener => listener(data))
  clearTimeout(poller.timer)
  if (data.files?.some(f => !f.ready) && poller.tries++ < RETRIES) {
    // eslint-disable-next-line no-use-before-define
    poller.timer = setTimeout(() => load(hash), RETRY_MS)
  }
}

const load = hash =>
  axios
    .post(homelabAudioHost(), { action: 'tracks', hash })
    .then(({ data }) => publish(hash, data))
    .catch(() => {}) // an old server or the torrent is not loaded: nothing to show

// { files: [{id, path, tracks, picked, how, ready}], choice: {tracks: [{name, lang}]} | null }
export function useHomelabAudio(hash) {
  const [data, setData] = useState(() => pollers.get(hash)?.data || null)

  useEffect(() => {
    if (!hash) return undefined
    let poller = pollers.get(hash)
    if (!poller) {
      poller = { data: null, listeners: new Set(), timer: null, tries: 0 }
      pollers.set(hash, poller)
      load(hash)
    } else if (poller.data) {
      setData(poller.data)
    }
    poller.listeners.add(setData)
    return () => {
      poller.listeners.delete(setData)
      if (!poller.listeners.size) {
        clearTimeout(poller.timer)
        pollers.delete(hash)
      }
    }
  }, [hash])

  return data
}

// the same dub under slightly different names ("LostFilm.TV" / "lostfilm tv") — as the server compares them
export const normName = name => (name || '').toLowerCase().replace(/[^\p{L}\p{N}]/gu, '')
export const trackKey = track => `${normName(track.name)}|${(track.lang || '').toLowerCase()}`

const post = (hash, body) =>
  axios
    .post(homelabAudioHost(), { hash, ...body })
    .then(({ data }) => publish(hash, data))
    .catch(err => {
      throw new Error(err?.response?.data?.error || err.message)
    })

// the torrent's tracks in order of preference (none — all tracks, as in the file); own tracks of episodes stay
export const homelabAudioSave = (hash, tracks) =>
  post(hash, { action: 'set', tracks: tracks.map(({ name, lang }) => ({ name, lang })) })

// a track for one episode (by the file path); null — back to the torrent's choice
export const homelabAudioSetFile = (hash, path, track) =>
  post(
    hash,
    track
      ? { action: 'file', path, track: { name: track.name, lang: track.lang, index: track.index } }
      : { action: 'file', path, reset: true },
  )

// every episode back to the torrent's choice
export const homelabAudioClearFiles = hash => post(hash, { action: 'files_clear' })

// what the player gets in a file: the chosen track, or the file as is — its default track
export function playingTrack(file) {
  if (!file?.ready || !file.tracks.length) return null
  if (file.picked >= 0) return file.tracks[file.picked]
  return file.tracks.find(tr => tr.default) || file.tracks[0]
}
