import type { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode, Ref } from 'react'
import { Link } from 'react-router-dom'
import { Tooltip } from './Tooltip'

// Buttons. The classes are the contract (themes style .btn), so a plain
// <button className="btn primary"> and <Button variant="primary"> are the same
// thing; the components add what's easy to forget: type="button", the busy
// state and a required label on anything that shows only an icon.

export type ButtonVariant = 'default' | 'primary' | 'ghost' | 'danger'
export type ButtonSize = 'sm' | 'md' | 'big'
type Look = { variant?: ButtonVariant; size?: ButtonSize; icon?: ReactNode }

export const btnClass = (variant: ButtonVariant = 'default', size: ButtonSize = 'md', extra?: string) =>
  ['btn', variant !== 'default' && variant, size !== 'md' && size, extra].filter(Boolean).join(' ')

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & Look & { busy?: boolean; ref?: Ref<HTMLButtonElement> }

export function Button({ variant, size, icon, busy, className, children, disabled, type = 'button', ...rest }: ButtonProps) {
  return (
    <button {...rest} type={type} className={btnClass(variant, size, className)} disabled={disabled || busy} aria-busy={busy || undefined}>
      {busy ? <span className="ui-btn-spin" aria-hidden="true" /> : icon}
      {children}
    </button>
  )
}

// A link that looks like a button: for going somewhere, never for doing something.
export function LinkButton({ to, variant, size, icon, className, children, ...rest }: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> & Look & { to: string }) {
  return <Link {...rest} to={to} className={btnClass(variant, size, className)}>{icon}{children}</Link>
}

// An icon on its own. label is required: it is the button's name for a screen
// reader and the tooltip for everyone else, on hover and on focus alike.
type IconButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label' | 'title'> & {
  label: string; variant?: ButtonVariant; size?: ButtonSize; round?: boolean; hideTip?: boolean; ref?: Ref<HTMLButtonElement>
}

export function IconButton({ label, variant = 'ghost', size = 'md', round, hideTip, className, children, type = 'button', ...rest }: IconButtonProps) {
  const button = <button {...rest} type={type} aria-label={label} className={btnClass(variant, size, (round ? 'icon' : 'square') + (className ? ' ' + className : ''))}>{children}</button>
  return hideTip ? button : <Tooltip text={label} describes={false}>{button}</Tooltip>
}
