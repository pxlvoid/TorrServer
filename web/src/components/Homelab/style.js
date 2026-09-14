// homelab: styles of the disk cache dialog — cards in the look of TorrentCard (theme.torrentCard colors).
// `dark` comes from the MUI palette: the styled theme has no mode flag, and light torrentCard colors are
// saturated green meant for white text — on the light dialog cards are white instead.
import styled, { css } from 'styled-components'

export const Section = styled.div`
  margin-bottom: 16px;
`

export const Bar = styled.div`
  ${({ theme: { torrentCard, secondary }, value, thin, dark }) => css`
    height: ${thin ? 4 : 6}px;
    border-radius: 3px;
    background: ${dark ? torrentCard.cardSecondaryColor : 'rgb(0 0 0 / 10%)'};
    overflow: hidden;

    ::after {
      content: '';
      display: block;
      height: 100%;
      width: ${Math.max(0, Math.min(100, value || 0))}%;
      min-width: ${value > 0 ? 3 : 0}px;
      background: ${secondary};
      border-radius: 3px;
    }
  `}
`

export const UsageRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 8px;

  .usage-main {
    font-size: 20px;
    font-weight: 300;
  }

  .usage-side {
    font-size: 12px;
    opacity: 0.7;
    text-align: right;
  }
`

export const SettingsRow = styled.div`
  ${({ theme: { torrentCard }, dark }) => css`
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px 12px;
    padding: 6px 6px 6px 12px;
    border-radius: 5px;
    background: ${dark ? torrentCard.cardPrimaryColor : '#fff'};

    .settings-summary {
      flex: 1;
      min-width: 160px;
      font-size: 14px;
    }

    .settings-summary small {
      display: block;
      opacity: 0.7;
      font-size: 12px;
    }
  `}
`

export const SettingsForm = styled.div`
  padding: 8px 4px 0;

  .settings-fields {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 16px;
    margin-top: 8px;
  }
`

export const CardList = styled.div`
  display: grid;
  gap: 10px;
`

export const Card = styled.div`
  ${({ theme: { torrentCard, secondary }, pinned, dark }) => css`
    display: grid;
    grid-template-columns: 64px 1fr auto;
    gap: 12px;
    padding: 10px;
    border-radius: 5px;
    background: ${dark ? torrentCard.cardPrimaryColor : '#fff'};
    box-shadow: 0 1px 3px rgb(0 0 0 / 20%);
    border-left: 3px solid ${pinned ? secondary : 'transparent'};

    @media (max-width: 500px) {
      grid-template-columns: 56px 1fr auto;
      gap: 10px;
    }
  `}
`

export const Poster = styled.div`
  ${({ theme: { torrentCard } }) => css`
    position: relative;
    width: 64px;
    height: 96px;
    border-radius: 5px;
    overflow: hidden;
    background: ${torrentCard.cardSecondaryColor};
    display: grid;
    place-items: center;

    img {
      width: 100%;
      height: 100%;
      object-fit: cover;
    }

    @media (max-width: 500px) {
      width: 56px;
      height: 84px;
    }
  `}
`

export const PlayingBadge = styled.div`
  ${({ theme: { secondary, torrentCard }, dark }) => css`
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 2px 0;
    font-size: 9px;
    font-weight: 600;
    letter-spacing: 0.4px;
    text-transform: uppercase;
    text-align: center;
    background: ${secondary};
    color: ${dark ? torrentCard.accentCardColor : '#fff'};
  `}
`

export const Info = styled.div`
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;

  .card-title {
    font-size: 15px;
    font-weight: 600;
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    word-break: break-word;
  }

  .card-subtitle {
    font-size: 12px;
    opacity: 0.75;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-stats {
    margin-top: auto;
    font-size: 12px;
    opacity: 0.85;
  }
`

export const Actions = styled.div`
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  margin: -6px -6px -6px 0;
`

// ---------- main screen: summary above the torrent list ----------

export const Summary = styled.div`
  ${({ theme: { torrentCard } }) => css`
    grid-column: 1 / -1; /* the torrent list is a grid: span the whole first row */
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 6px 14px;
    padding: 12px 16px;
    border-radius: 5px;
    background: ${torrentCard.cardPrimaryColor};
    color: #fff;
    box-shadow: 0px 2px 4px -1px rgb(0 0 0 / 20%), 0px 4px 5px 0px rgb(0 0 0 / 14%), 0px 1px 10px 0px rgb(0 0 0 / 12%);
    cursor: pointer;
    transition: filter 0.2s;

    :hover {
      filter: brightness(1.08);
    }

    .summary-icon {
      grid-row: span 2;
      opacity: 0.9;
    }

    .summary-main {
      font-size: 15px;
      min-width: 0;
    }

    .summary-main b {
      font-weight: 600;
    }

    .summary-side {
      font-size: 12px;
      opacity: 0.85;
      text-align: right;
      white-space: nowrap;
    }

    .summary-bar {
      grid-column: 2 / -1;
    }

    .summary-playing {
      grid-column: 2 / -1;
      font-size: 12px;
      opacity: 0.9;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    @media (max-width: 700px) {
      padding: 10px 12px;
      grid-template-columns: auto 1fr;

      .summary-side {
        grid-column: 2;
        text-align: left;
      }
    }
  `}
`

export const SummaryBar = styled.div`
  height: 4px;
  border-radius: 2px;
  background: rgb(255 255 255 / 25%);
  overflow: hidden;

  ::after {
    content: '';
    display: block;
    height: 100%;
    width: ${({ value }) => Math.max(0, Math.min(100, value || 0))}%;
    background: #fff;
  }
`

// ---------- main screen: badge on the torrent card poster ----------

export const PosterBadge = styled.div`
  position: absolute;
  inset: 0;
  pointer-events: none;

  /* pill in the top left corner: posters usually carry their title at the bottom */
  .badge-label {
    position: absolute;
    top: 4px;
    left: 4px;
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 3px 6px 3px 4px;
    border-radius: 10px;
    background: rgb(0 0 0 / 65%);
    color: #fff;
    font-size: 11px;
    font-weight: 600;
    line-height: 1;
  }

  /* TorrentCardPoster styles every svg inside it (width: 50%, translateY) — not ours */
  .badge-label svg {
    font-size: 13px;
    width: 1em !important;
    height: 1em;
    transform: none !important;
  }

  /* share on disk as a strip along the bottom edge */
  .badge-bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 4px;
    background: rgb(0 0 0 / 45%);
  }

  .badge-bar div {
    height: 100%;
    background: #fff;
  }

  @media (max-width: 770px) {
    .badge-label {
      top: 3px;
      left: 3px;
      padding: 2px 4px 2px 3px;
      font-size: 9px;
    }

    .badge-label svg {
      font-size: 10px;
    }

    .badge-bar {
      height: 3px;
    }
  }
`

export const PinMark = styled.div`
  position: absolute;
  top: 4px;
  right: 4px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: rgb(0 0 0 / 55%);
  color: #ffd54f;
  pointer-events: none;

  svg {
    font-size: 14px;
    width: 1em !important;
    height: 1em;
    transform: none !important;
  }

  @media (max-width: 770px) {
    width: 16px;
    height: 16px;

    svg {
      font-size: 11px;
    }
  }
`

// ---------- torrent details: what of the torrent is on disk ----------

export const Timeline = styled.div`
  margin-bottom: 8px;

  .timeline-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 10px;
    font-size: 14px;
  }

  .timeline-head b {
    font-size: 18px;
    font-weight: 600;
  }

  .timeline-head span {
    opacity: 0.75;
    font-size: 12px;
    text-align: right;
  }

  canvas {
    display: block;
    width: 100%;
    height: 36px;
    border-radius: 4px;
  }

  .timeline-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin-top: 10px;
    font-size: 12px;
    opacity: 0.85;
  }

  .timeline-legend i {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 2px;
    margin-right: 5px;
    vertical-align: -1px;
  }
`
