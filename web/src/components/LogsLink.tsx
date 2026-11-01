import { Link } from 'react-router-dom'
import { useApp } from '../state'

// The way from an error to something to send someone: the log download, sanitized unless
// they say otherwise. Only an administrator can get there, so only they see it.
export default function LogsLink({ className = 'btn ghost' }: { className?: string }) {
  const app = useApp()
  if (!app?.user?.isAdmin) return null
  return <Link className={className} to="/settings/logs?tab=bundle">Download logs</Link>
}
