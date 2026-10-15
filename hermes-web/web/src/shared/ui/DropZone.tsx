import { useId, useState } from 'react'
import type { DragEvent } from 'react'
import { UploadIcon } from './icons'
import './ui.css'

/**
 * A file drop target that is also a file button. Dropping and choosing both call onFile with the
 * first file; checking its type and size is the caller's job (the browser's accept is only a hint).
 */
export function DropZone({
  onFile,
  accept,
  prompt,
  button,
  hint,
}: {
  onFile: (file: File) => void
  accept?: string
  prompt: string
  button: string
  hint?: string
}) {
  const id = useId()
  const [over, setOver] = useState(false)

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setOver(false)
    const file = e.dataTransfer.files[0]
    if (file) onFile(file)
  }

  return (
    <div
      className={over ? 'drop-zone drop-zone-over' : 'drop-zone'}
      onDragOver={(e) => {
        e.preventDefault()
        setOver(true)
      }}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
    >
      <UploadIcon size={22} className="drop-zone-icon" />
      <span className="drop-zone-prompt">{prompt}</span>
      <label htmlFor={id} className="btn btn-secondary drop-zone-button">
        {button}
      </label>
      <input
        id={id}
        type="file"
        accept={accept}
        className="visually-hidden"
        onChange={(e) => {
          const file = e.target.files?.[0]
          if (file) onFile(file)
          e.target.value = '' // so choosing the same file again still fires
        }}
      />
      {hint && <span className="muted small">{hint}</span>}
    </div>
  )
}
