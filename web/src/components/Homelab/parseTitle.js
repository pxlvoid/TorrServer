// homelab: short card title from a tracker / Lampa torrent name.
//   "[LAMPA] Обсессия / Obsession (Карри Баркер / Curry Barker) [2025, США, ужасы, UHD BDRemux 2160p, HDR10] …"
//     → { title: 'Обсессия', subtitle: 'Obsession · 2025 · 2160p · HDR' }
//   "Severance.S02.1080p.WEB-DL" → { title: 'Severance', subtitle: 'S02 · 1080p' }

const TOKEN_START = /\s(?=S\d{1,2}(E\d{1,3})?\b|(19|20)\d{2}\b|\d{3,4}p\b)/i

export function parseTitle(raw) {
  const full = (raw || '').trim()
  let name = full.replace(/^\[(lampa|torrserver)\]\s*/i, '')

  // scene style "Name.S01.1080p.WEB-DL" — no spaces, dots instead; only if a season/year/quality token
  // follows, otherwise it is just a file name ("ubuntu-24.04.3-live-server-amd64.iso") and stays as is
  const spaced = name.replace(/[._]+/g, ' ')
  const dotted = !/\s/.test(name) && spaced.search(TOKEN_START) > 0
  if (dotted) name = spaced

  const cut = dotted ? name.search(TOKEN_START) : name.search(/\s[([]/)
  const head = (cut > 0 ? name.slice(0, cut) : name).trim()
  const tail = cut > 0 ? name.slice(cut) : ''
  const [title, altTitle] = head
    .split(/\s+\/\s*|\s*\/\s+/) // " / " between titles, but not AC/DC
    .map(part => part.trim())
    .filter(Boolean)

  const meta = []
  if (altTitle) meta.push(altTitle)
  const season = tail.match(/\bS(\d{1,2})(?:E\d{1,3})?\b/i) || tail.match(/сезон[:\s]*(\d{1,2})/i)
  if (season) meta.push(`S${season[1].padStart(2, '0')}`)
  const year = tail.match(/\b(19|20)\d{2}\b/)
  if (year) meta.push(year[0])
  const quality = tail.match(/\b(\d{3,4}p)\b/i)
  if (quality) meta.push(quality[1].toLowerCase())
  if (/\bHDR|Dolby Vision/i.test(tail)) meta.push('HDR')

  return { title: title || full, subtitle: meta.join(' · ') }
}
