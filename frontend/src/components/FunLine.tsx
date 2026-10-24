import { useFunLine } from '../lib/fun';
import type { Language } from '../lib/i18n';

export function FunLine({ language, className }: { language: Language; className?: string }) {
  const line = useFunLine(language);
  return <p className={className}>{line}</p>;
}
