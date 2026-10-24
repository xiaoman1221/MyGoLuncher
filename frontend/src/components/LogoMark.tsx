import { useId } from 'react';

export function LogoMark({ size = 40 }: { size?: number }) {
  const id = useId().replace(/:/g, '');
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      role="img"
      aria-label="MyGoLuncher"
      className="logo-mark"
    >
      <defs>
        <linearGradient id={`top-${id}`} x1="10" y1="10" x2="54" y2="38" gradientUnits="userSpaceOnUse">
          <stop style={{ stopColor: 'var(--logo-top-1)' }} />
          <stop offset="0.45" style={{ stopColor: 'var(--logo-top-2)' }} />
          <stop offset="1" style={{ stopColor: 'var(--logo-top-3)' }} />
        </linearGradient>
        <linearGradient id={`left-${id}`} x1="11" y1="24" x2="34" y2="58" gradientUnits="userSpaceOnUse">
          <stop style={{ stopColor: 'var(--logo-left-1)' }} />
          <stop offset="1" style={{ stopColor: 'var(--logo-left-2)' }} />
        </linearGradient>
        <linearGradient id={`right-${id}`} x1="53" y1="20" x2="33" y2="56" gradientUnits="userSpaceOnUse">
          <stop style={{ stopColor: 'var(--logo-right-1)' }} />
          <stop offset="1" style={{ stopColor: 'var(--logo-right-2)' }} />
        </linearGradient>
        <filter id={`glow-${id}`} x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="3" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <path d="M32 3.5L61 18.2L32 34.2L3 18.2L32 3.5Z" fill={`url(#top-${id})`} />
      <path d="M3 18.2L32 34.2V61.5L3 45.8V18.2Z" fill={`url(#left-${id})`} />
      <path d="M61 18.2L32 34.2V61.5L61 45.8V18.2Z" fill={`url(#right-${id})`} />
      <path d="M10 21.3L22 27.6L17 31.4L11.5 28.5Z" style={{ fill: 'var(--logo-left-2)' }} opacity="0.8" />
      <path d="M36 44.5L54 35.2V42.8L36 52.2V44.5Z" style={{ fill: 'var(--logo-top-3)' }} opacity="0.3" />
      <path d="M39 39L50 33.3V37L39 42.7V39Z" style={{ fill: 'var(--logo-top-1)' }} opacity="0.3" />
      <g filter={`url(#glow-${id})`}>
        <path d="M40.5 47.2L47.3 43.6V49.1L40.5 52.7V47.2Z" style={{ fill: 'var(--logo-top-1)' }} />
        <path d="M41.5 48L46.3 45.5V46.7L41.5 49.2V48Z" fill="#FFFFFF" opacity="0.75" />
      </g>
      <path d="M32 3.5L61 18.2L32 34.2" style={{ stroke: 'var(--logo-top-1)' }} strokeOpacity="0.32" />
    </svg>
  );
}
