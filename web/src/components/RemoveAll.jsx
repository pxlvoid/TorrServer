import { Button, Dialog, DialogActions, DialogTitle } from '@material-ui/core'
import ListItem from '@material-ui/core/ListItem'
import ListItemIcon from '@material-ui/core/ListItemIcon'
import ListItemText from '@material-ui/core/ListItemText'
import DeleteIcon from '@material-ui/icons/Delete'
import { useState } from 'react'
import { torrentsHost } from 'utils/Hosts'
import { useTranslation } from 'react-i18next'

import UnsafeButton from './UnsafeButton'
import { HomelabWipeOption, homelabRemoveAll } from './Homelab/WipeOption' // homelab

const fnRemoveAll = () => {
  fetch(torrentsHost(), {
    method: 'post',
    body: JSON.stringify({ action: 'wipe' }),
    headers: {
      Accept: 'application/json, text/plain, */*',
      'Content-Type': 'application/json',
    },
  })
}

export default function RemoveAll({ isOffline, isLoading }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const closeDialog = () => setOpen(false)
  const openDialog = () => setOpen(true)

  return (
    <>
      <ListItem disabled={isOffline || isLoading} button key={t('RemoveAll')} onClick={openDialog}>
        <ListItemIcon>
          <DeleteIcon />
        </ListItemIcon>

        <ListItemText primary={t('RemoveAll')} />
      </ListItem>

      <Dialog open={open} onClose={closeDialog}>
        <DialogTitle>{t('DeleteTorrents?')}</DialogTitle>
        <HomelabWipeOption /* homelab */ />
        <DialogActions>
          <Button variant='outlined' onClick={closeDialog} color='secondary'>
            {t('Cancel')}
          </Button>

          <UnsafeButton
            timeout={5}
            startIcon={<DeleteIcon />}
            variant='contained'
            onClick={() => {
              homelabRemoveAll(fnRemoveAll) // homelab: optionally clears the unpinned disk cache too
              closeDialog()
            }}
            color='secondary'
            autoFocus
          >
            {t('OK')}
          </UnsafeButton>
        </DialogActions>
      </Dialog>
    </>
  )
}
