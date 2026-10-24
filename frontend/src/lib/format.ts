export function cn(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(' ');
}

export function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return '0';
  if (value >= 100_000_000) return `${(value / 100_000_000).toFixed(value >= 1_000_000_000 ? 0 : 1)}亿`;
  if (value >= 10_000) return `${(value / 10_000).toFixed(value >= 1_000_000 ? 0 : 1)}万`;
  return new Intl.NumberFormat('zh-CN').format(value);
}

export function formatSpeed(bytesPerSecond: number): string {
  if (!bytesPerSecond) return '—';
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  let value = bytesPerSecond;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(value >= 100 || index === 0 ? 0 : 1)} ${units[index]}`;
}

export function formatBytes(bytes: number): string {
  if (!bytes) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = bytes;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index++;
  }
  return `${size.toFixed(size >= 100 || index === 0 ? 0 : 1)} ${units[index]}`;
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

export function formatDateTime(value: string | Date | undefined, locale = 'zh-CN'): string {
  if (!value) return '—';
  const date = typeof value === 'string' ? new Date(value) : value;
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
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

export function formatPlayTime(minutes: number): string {
  if (!minutes) return '0 分钟';
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.floor(minutes / 60);
  if (hours < 100) {
    const rest = minutes % 60;
    return rest ? `${hours} 小时 ${rest} 分` : `${hours} 小时`;
  }
  return `${Math.round(hours)} 小时`;
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

export function accentFromString(value: string): string {
  let hash = 0;
  for (let index = 0; index < value.length; index++) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  const hue = 78 + (hash % 110);
  return `hsl(${hue} 55% 45%)`;
}

export function greetingKey(): 'greeting.late' | 'greeting.morning' | 'greeting.noon' | 'greeting.afternoon' | 'greeting.evening' {
  const hour = new Date().getHours();
  if (hour < 6) return 'greeting.late';
  if (hour < 11) return 'greeting.morning';
  if (hour < 14) return 'greeting.noon';
  if (hour < 19) return 'greeting.afternoon';
  return 'greeting.evening';
}

export function stageLabel(stage: string): string {
  const key = stage.includes(':') ? stage.split(':').pop()! : stage;
  switch (key) {
    case 'game':
      return '游戏文件';
    case 'loader':
      return '加载器';
    case 'java':
      return 'Java';
    case 'download':
      return '下载';
    case 'assets':
      return '资源文件';
    case 'libraries':
      return '运行库';
    case 'import':
      return '导入';
    case 'extract':
      return '解压';
    case 'done':
      return '完成';
    case 'mirror':
      return '镜像测速';
    case 'error':
      return '失败';
    default:
      return '处理中';
  }
}
