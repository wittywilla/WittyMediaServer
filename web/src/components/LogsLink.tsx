// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { useApp } from '../state'

// The way from an error to something to send someone: the log download, sanitized unless
// they say otherwise. Only an administrator can get there, so only they see it.
export default function LogsLink({ className = 'btn ghost' }: { className?: string }) {
  const app = useApp()
  if (!app?.user?.isAdmin) return null
  return <Link className={className} to="/settings/logs?tab=bundle">Download logs</Link>
}
