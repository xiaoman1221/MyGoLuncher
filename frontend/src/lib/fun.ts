import { useEffect, useState } from 'react';
import { translate, type Language, type MessageKey } from './i18n';

const loadingKeys: MessageKey[] = ['fun.loadingOne', 'fun.loadingTwo', 'fun.loadingThree'];
const mygoKeys: MessageKey[] = ['fun.mygoHaruhikage', 'fun.mygoSlogan'];

/** Rotates a small loading line. Every third tick is a MyGO easter egg. */
export function useFunLine(language: Language, intervalMs = 3200): string {
  const [tick, setTick] = useState(() => Math.floor(Math.random() * 6));
  useEffect(() => {
    const timer = window.setInterval(() => {
      setTick((current) => (current + 1) % 6);
    }, intervalMs);
    return () => window.clearInterval(timer);
  }, [intervalMs]);
  const showMygo = tick % 3 === 2;
  const key = showMygo
    ? mygoKeys[Math.floor(tick / 3) % mygoKeys.length]
    : loadingKeys[tick % loadingKeys.length];
  return translate(language, key);
}
