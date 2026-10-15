import { createLink } from '@tanstack/react-router'
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, Ref } from 'react'
import './ui.css'

/** primary: the one main action on a screen (ink). secondary: other actions. ghost: inline text actions. */
export type ButtonVariant = 'primary' | 'secondary' | 'ghost'

const classes = (variant: ButtonVariant, className?: string) =>
  ['btn', `btn-${variant}`, className].filter(Boolean).join(' ')

export function Button({
  variant = 'secondary',
  className,
  type = 'button',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant }) {
  return <button type={type} className={classes(variant, className)} {...props} />
}

function ButtonAnchor({
  variant = 'secondary',
  className,
  ref,
  ...props
}: AnchorHTMLAttributes<HTMLAnchorElement> & { variant?: ButtonVariant; ref?: Ref<HTMLAnchorElement> }) {
  return <a ref={ref} className={classes(variant, className)} {...props} />
}

/** A router link that looks like a button, for actions that navigate. */
export const ButtonLink = createLink(ButtonAnchor)
