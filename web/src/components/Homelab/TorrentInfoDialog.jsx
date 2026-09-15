// homelab: the standard "Torrent info" window (DialogTorrentDetailsContent), opened from the disk cache dialog.
// The torrent card passes it the torrent from the list polled every second; here the torrent is asked for itself
// every second — the window waits for its state to leave "in the list / getting info", a single snapshot would
// keep it loading forever.
import { forwardRef, useEffect, useState } from 'react'
import Slide from '@material-ui/core/Slide'
import { useMediaQuery, useTheme } from '@material-ui/core'
import axios from 'axios'
import DialogTorrentDetailsContent from 'components/DialogTorrentDetailsContent'
import { StyledDialog } from 'style/CustomMaterialUiStyles'
import { torrentsHost } from 'utils/Hosts'
import useOnStandaloneAppOutsideClick from 'utils/useOnStandaloneAppOutsideClick'

const Transition = forwardRef((props, ref) => <Slide direction='up' ref={ref} {...props} />)
const REFRESH_MS = 1000

export default function HomelabTorrentInfoDialog({ hash, handleClose }) {
  const theme = useTheme()
  const fullScreen = useMediaQuery(theme.breakpoints.down('md'))
  const ref = useOnStandaloneAppOutsideClick(handleClose)
  const [torrent, setTorrent] = useState(null)

  useEffect(() => {
    let alive = true
    const load = () =>
      axios
        .post(torrentsHost(), { action: 'get', hash })
        .then(({ data }) => alive && data && setTorrent(data))
        .catch(() => {})
    load()
    const id = setInterval(load, REFRESH_MS)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [hash])

  return (
    <StyledDialog
      open
      onClose={handleClose}
      fullScreen={fullScreen}
      fullWidth
      maxWidth='xl'
      TransitionComponent={Transition}
      ref={ref}
    >
      {torrent && <DialogTorrentDetailsContent closeDialog={handleClose} torrent={torrent} />}
    </StyledDialog>
  )
}
