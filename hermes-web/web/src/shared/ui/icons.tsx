import type { SVGProps } from 'react'

// Inline stroke icons, drawn on a 16-unit grid in currentColor. All are decorative (aria-hidden):
// the text next to them carries the meaning.
function Icon({ size = 16, strokeWidth = 1.5, children, ...props }: SVGProps<SVGSVGElement> & { size?: number }) {
  return (
    <svg
      viewBox="0 0 16 16"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      {children}
    </svg>
  )
}

type IconProps = SVGProps<SVGSVGElement> & { size?: number }

export const CheckIcon = (p: IconProps) => <Icon strokeWidth={2} {...p}><path d="M3.5 8.5l3 3 6-7" /></Icon>
export const CrossIcon = (p: IconProps) => <Icon strokeWidth={2} {...p}><path d="M5 5l6 6M11 5l-6 6" /></Icon>
export const ChevronRightIcon = (p: IconProps) => <Icon {...p}><path d="M6 3.5L10.5 8 6 12.5" /></Icon>
export const UploadIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M8 11V2.5" />
    <path d="M4.5 6L8 2.5 11.5 6" />
    <path d="M2.5 11v2.5h11V11" />
  </Icon>
)
export const FileIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M4 1.5h5l3 3v10H4z" />
    <path d="M9 1.5v3h3" />
  </Icon>
)
export const CopyIcon = (p: IconProps) => (
  <Icon {...p}>
    <rect x="5" y="5" width="8.5" height="8.5" rx="1" />
    <path d="M3 10.5V2.5h8" />
  </Icon>
)
export const InfoIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="8" cy="8" r="6.5" />
    <path d="M8 7.5v4" />
    <path d="M8 5v.01" />
  </Icon>
)
export const WarnIcon = (p: IconProps) => (
  <Icon {...p}>
    <path d="M8 2l6.5 11.5h-13z" />
    <path d="M8 6.5v3" />
    <path d="M8 11.5v.01" />
  </Icon>
)
export const ErrorIcon = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="8" cy="8" r="6.5" />
    <path d="M5.75 5.75l4.5 4.5M10.25 5.75l-4.5 4.5" />
  </Icon>
)
/** The Hermes mark: an H, drawn for the 24 px logo tile. */
export const LogoMark = (p: IconProps) => <Icon size={14} strokeWidth={2} {...p}><path d="M5 4v8M11 4v8M5 8h6" /></Icon>
