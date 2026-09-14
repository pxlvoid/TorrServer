// homelab: sidebar entry of the persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
import ListItemIcon from '@material-ui/core/ListItemIcon'
import ListItemText from '@material-ui/core/ListItemText'
import StorageIcon from '@material-ui/icons/Storage'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { StyledMenuButtonWrapper } from 'style/CustomMaterialUiStyles'
import { isStandaloneApp } from 'utils/Utils'

import './i18n'
import DiskCacheDialog from './DiskCacheDialog'

export default function HomelabDiskCacheButton({ isOffline, isLoading }) {
  const { t } = useTranslation()
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  return (
    <div>
      <StyledMenuButtonWrapper disabled={isOffline || isLoading} button onClick={() => setIsDialogOpen(true)}>
        {isStandaloneApp ? (
          <>
            <StorageIcon />
            <div>{t('Homelab.DiskCache')}</div>
          </>
        ) : (
          <>
            <ListItemIcon>
              <StorageIcon />
            </ListItemIcon>

            <ListItemText primary={t('Homelab.DiskCache')} />
          </>
        )}
      </StyledMenuButtonWrapper>

      {isDialogOpen && <DiskCacheDialog handleClose={() => setIsDialogOpen(false)} />}
    </div>
  )
}
