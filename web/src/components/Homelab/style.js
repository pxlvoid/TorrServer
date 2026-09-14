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
