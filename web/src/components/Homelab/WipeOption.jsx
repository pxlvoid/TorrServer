// homelab: "Remove all" and the persistent disk cache (hooks in RemoveAll). Removing torrents from the list keeps
// their disk cache in persistent mode; a checkbox in the confirmation clears the unpinned cache too (pinned and
// downloaded stays). Off by default — the cache stays, as for every removal.
import { Checkbox, DialogContent, FormControlLabel } from '@material-ui/core'
import axios from 'axios'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { torrentsHost } from 'utils/Hosts'
import { humanizeSize } from 'utils/Utils'

import './i18n'
import { homelabCacheHost, refreshHomelabCache, useHomelabCache } from './store'

let clearCache = false // the checkbox of the open dialog, read by homelabRemoveAll

export function HomelabWipeOption() {
  const { t } = useTranslation()
  const data = useHomelabCache()
  const [checked, setChecked] = useState(false)

  useEffect(() => {
    clearCache = false // every dialog starts unchecked
    return () => {
      clearCache = false
    }
  }, [])

  if (!data?.usage?.enabled) return null
  const unpinned = (data.items || []).filter(it => !it.pinned).reduce((sum, it) => sum + it.size, 0)
  if (!unpinned) return null

  return (
    <DialogContent>
      <FormControlLabel
        control={
          <Checkbox
            checked={checked}
            onChange={e => {
              setChecked(e.target.checked)
              clearCache = e.target.checked
            }}
          />
        }
        label={t('Homelab.WipeClearCache', { size: humanizeSize(unpinned) })}
      />
    </DialogContent>
  )
}

// replaces the upstream wipe call: with the checkbox, removes the torrents and then clears the unpinned cache
export function homelabRemoveAll(upstreamWipe) {
  if (!clearCache) return upstreamWipe()
  clearCache = false
  return axios
    .post(torrentsHost(), { action: 'wipe' })
    .then(() => axios.post(homelabCacheHost(), { action: 'clear' }))
    .then(() => refreshHomelabCache())
    .catch(() => {})
}
