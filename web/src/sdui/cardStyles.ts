// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The fourteen card styles (docs/SDUI.md, "Card styles"): what each shows of what a
// card carries, in reading order, and how many lines its title and subtitle may take.
// A style this table doesn't have is drawn as its shape: title and subtitle under the picture.

export type CardStyle = { shape: string; aspect: string; shows: string[]; titleLines: number; subtitleLines: number }

const style = (shape: string, aspect: string, shows: string, titleLines: number, subtitleLines: number): CardStyle =>
  ({ shape, aspect, shows: shows ? shows.split(' ') : [], titleLines, subtitleLines })

export const CARD_STYLES: Record<string, CardStyle> = {
  poster: style('poster', '2/3', 'title subtitle progress watched unwatched badges', 1, 1),
  landscape: style('landscape', '16/9', 'title subtitle badges', 1, 1),
  progress: style('landscape', '16/9', 'title subtitle progress remaining', 1, 1),
  episode: style('landscape', '16/9', 'title subtitle synopsis progress watched', 1, 1),
  square: style('square', '1/1', 'title subtitle', 1, 1),
  album: style('square', '1/1', 'title subtitle', 1, 1),
  circle: style('circle', '1/1', 'title', 2, 0),
  person: style('circle', '1/1', 'title subtitle badges', 1, 1),
  book: style('poster', '2/3', 'title subtitle progress', 2, 1),
  game: style('poster', '3/4', 'title subtitle badges', 1, 1),
  photo: style('square', '1/1', '', 0, 0),
  channel: style('landscape', '16/9', 'number title now next progress', 1, 2),
  tile: style('landscape', '16/9', 'title count', 1, 0),
  plate: style('poster', '2/3', 'title subtitle badges', 2, 1),
  block: style('landscape', '16/9', 'title subtitle', 2, 1),
  logo: style('landscape', '16/9', 'number title', 1, 0),
  chip: style('chip', '', 'title', 1, 0),
}

export const UNKNOWN_STYLE: CardStyle = style('poster', '2/3', 'title subtitle', 1, 1)

export const cardStyle = (name: unknown): CardStyle | undefined => (typeof name === 'string' ? CARD_STYLES[name] : undefined)
