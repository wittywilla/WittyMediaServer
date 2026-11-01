import { Children, cloneElement, isValidElement, useId, type InputHTMLAttributes, type ReactElement, type ReactNode, type Ref, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'

// Form fields. Field joins a label, a hint and an error to its control, so the
// control is named and described for a screen reader without any ids by hand.

type Described = { id?: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean | 'true' | 'false'; 'aria-required'?: boolean }

export function Field({ label, hint, error, required, children, className }: { label: ReactNode; hint?: ReactNode; error?: ReactNode; required?: boolean; children: ReactNode; className?: string }) {
  const id = useId()
  const hintId = hint ? id + 'h' : undefined, errorId = error ? id + 'e' : undefined
  const only = Children.count(children) === 1 && isValidElement(children) ? children as ReactElement<Described> : null
  const describedBy = [only?.props['aria-describedby'], hintId, errorId].filter(Boolean).join(' ') || undefined
  return (
    <div className={'ui-field' + (error ? ' invalid' : '') + (className ? ' ' + className : '')}>
      <label className="ui-field-label" htmlFor={only ? only.props.id || id : undefined} id={id + 'l'}>
        {label}{required && <span className="ui-field-req"> (required)</span>}
      </label>
      {only
        ? cloneElement(only, { id: only.props.id || id, 'aria-describedby': describedBy, 'aria-invalid': error ? true : only.props['aria-invalid'], 'aria-required': required || undefined })
        : <div role="group" aria-labelledby={id + 'l'} aria-describedby={describedBy}>{children}</div>}
      {hint && <div className="hint" id={hintId}>{hint}</div>}
      {error && <div className="ui-field-error" id={errorId} role="alert">{error}</div>}
    </div>
  )
}

const size = (s?: 'sm') => (s ? ' sm' : '')

export function Input({ className, inputSize, ...rest }: InputHTMLAttributes<HTMLInputElement> & { inputSize?: 'sm'; ref?: Ref<HTMLInputElement> }) {
  return <input {...rest} className={'input' + size(inputSize) + (className ? ' ' + className : '')} />
}

export function Textarea({ className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { ref?: Ref<HTMLTextAreaElement> }) {
  return <textarea {...rest} className={'input' + (className ? ' ' + className : '')} />
}

// Select is the browser's own list, which every device already knows how to
// show well, in the design's frame. options is the short form of the children.
export function Select({ className, inputSize, options, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { inputSize?: 'sm'; options?: { value: string; label: string; disabled?: boolean }[]; ref?: Ref<HTMLSelectElement> }) {
  return (
    <select {...rest} className={'select' + size(inputSize) + (className ? ' ' + className : '')}>
      {options?.map((o) => <option key={o.value} value={o.value} disabled={o.disabled}>{o.label}</option>)}
      {children}
    </select>
  )
}

export function Switch({ checked, onChange, label, disabled, labelledBy }: { checked: boolean; onChange: (v: boolean) => void; label?: string; disabled?: boolean; labelledBy?: string }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={labelledBy ? undefined : label} aria-labelledby={labelledBy} disabled={disabled}
      className={'set-switch' + (checked ? ' on' : '')} onClick={() => onChange(!checked)}><i /></button>
  )
}
