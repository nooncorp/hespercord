import { useState } from 'react'

type Size = 'sm' | 'md' | 'lg' | 'xl'

const sizeClasses: Record<Size, string> = {
  sm: 'w-6 h-6 text-[10px]',
  md: 'w-8 h-8 text-xs',
  lg: 'w-10 h-10 text-sm',
  xl: 'w-16 h-16 text-2xl',
}

interface AvatarProps {
  url: string | null | undefined
  letter: string
  size?: Size
  className?: string
}

export default function Avatar({ url, letter, size = 'md', className = '' }: AvatarProps) {
  const [imgError, setImgError] = useState(false)
  const showImg = url && !imgError

  const baseClass = `rounded-full flex items-center justify-center font-bold text-white shrink-0 ${sizeClasses[size]} ${className}`

  if (showImg) {
    return (
      <img
        src={url}
        alt=""
        className={`rounded-full object-cover shrink-0 ${sizeClasses[size]} ${className}`}
        onError={() => setImgError(true)}
      />
    )
  }

  return (
    <div className={`${baseClass} bg-bg-accent`}>
      {letter.charAt(0).toUpperCase()}
    </div>
  )
}
