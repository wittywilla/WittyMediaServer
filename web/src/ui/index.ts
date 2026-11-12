// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The shared parts. docs/DESIGN.md says what each is for; /design shows them all
// on one page, in whatever theme is in use, for administrators.
export { Button, IconButton, LinkButton, btnClass, type ButtonSize, type ButtonVariant } from './Button'
export { Tooltip } from './Tooltip'
export { Field, Input, Select, Switch, Textarea } from './Field'
export { Chip, Chips, Segmented, Tabs, type Option, type TabItem } from './Choice'
export { AskHost, Dialog, Sheet, ask, useBackCloses } from './Overlay'
export { Menu, type MenuEntry } from './Menu'
export { ToastHost, toast, type ToastOptions } from './Toast'
export { Card, CardRow, EmptyState, LoadError, Notice, PageSkeleton, PosterCard, Progress, ProgressRing, Shelf, Skeleton, SkeletonGrid, Spinner, shapeFor, type PageShape, type PosterShape } from './Surface'
export { RowSheet, Rows, SheetRow } from './RowSheet'
export { DataTable, type Column } from './DataTable'
export { useUserPref, setUserPref, getUserPref } from './userPrefs'
export { Dock, type DockAction } from './Dock'
export { KeyLegend } from './KeyLegend'
export { KeyHelp } from './KeyHelp'
