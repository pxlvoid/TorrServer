// homelab: episode labels of a torrent's files — the episode number parsed from the name (season.episode if the
// torrent has several seasons), the position if there is none. Shared by the download panel and the audio choice.
import ptt from 'parse-torrent-title'

export const VIDEO_EXT = /\.(3gp|avi|divx|flv|m2ts|m4v|mkv|mov|mp4|mpe?g|mts|ogv|rmvb|ts|vob|webm|wmv)$/i

export function labelEpisodes(files) {
  const parsed = files.map((f, i) => {
    const { season, episode } = ptt.parse(f.path.split('/').pop())
    return { ...f, season, episode: episode ?? i + 1 }
  })
  const seasons = new Set(parsed.map(e => e.season).filter(Boolean))
  return parsed.map(e => ({ ...e, label: seasons.size > 1 && e.season ? `${e.season}.${e.episode}` : `${e.episode}` }))
}
