export function cn(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(' ');
}

export function formatCompactNumber(value: number, locale = 'zh-CN'): string {
  if (!Number.isFinite(value)) return '0';
  return new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}

export function formatDate(value: string | Date | undefined, locale = 'zh-CN', fallback = '—'): string {
  if (!value) return fallback;
  const date = typeof value === 'string' ? new Date(value) : value;
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 1971) return fallback;
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date);
}

export function relativeTime(value: string | undefined, locale = 'zh-CN', fallback = '—'): string {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 1971) return fallback;
  const diff = date.getTime() - Date.now();
  const abs = Math.abs(diff);
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (abs < 60_000) return formatter.format(0, 'second');
  if (abs < 3_600_000) return formatter.format(Math.round(diff / 60_000), 'minute');
  if (abs < 86_400_000) return formatter.format(Math.round(diff / 3_600_000), 'hour');
  if (abs < 2_592_000_000) return formatter.format(Math.round(diff / 86_400_000), 'day');
  if (abs < 31_536_000_000) return formatter.format(Math.round(diff / 2_592_000_000), 'month');
  return formatter.format(Math.round(diff / 31_536_000_000), 'year');
}

export function loaderLabel(loader: string, vanillaLabel = '原版'): string {
  switch (loader.toLowerCase()) {
    case 'fabric':
      return 'Fabric';
    case 'quilt':
      return 'Quilt';
    case 'forge':
      return 'Forge';
    case 'neoforge':
      return 'NeoForge';
    default:
      return vanillaLabel;
  }
}

export function loaderTone(loader: string): string {
  switch (loader.toLowerCase()) {
    case 'fabric':
      return 'fabric';
    case 'quilt':
      return 'quilt';
    case 'forge':
      return 'forge';
    case 'neoforge':
      return 'neoforge';
    default:
      return 'vanilla';
  }
}

export function initials(name: string): string {
  const cleaned = name.trim();
  if (!cleaned) return 'MC';
  const latin = cleaned.match(/[A-Za-z0-9]/g);
  if (latin && latin.length >= 2) return (latin[0] + latin[1]).toUpperCase();
  return Array.from(cleaned).slice(0, 1).join('');
}

export function greetingKey(): 'greeting.late' | 'greeting.morning' | 'greeting.noon' | 'greeting.afternoon' | 'greeting.evening' {
  const hour = new Date().getHours();
  if (hour < 6) return 'greeting.late';
  if (hour < 11) return 'greeting.morning';
  if (hour < 14) return 'greeting.noon';
  if (hour < 19) return 'greeting.afternoon';
  return 'greeting.evening';
}
