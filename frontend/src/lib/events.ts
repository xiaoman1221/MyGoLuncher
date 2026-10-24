import { isDesktop } from './api';
import { mockSubscribe } from './mock';

type WailsWindow = Window & {
  runtime?: {
    EventsOn?: (name: string, callback: (payload: unknown) => void) => () => void;
  };
};

export function onEvent<T>(event: string, handler: (payload: T) => void): () => void {
  const runtime = (window as WailsWindow).runtime;
  if (isDesktop && runtime?.EventsOn) {
    return runtime.EventsOn(event, (payload) => handler(payload as T));
  }
  return mockSubscribe(event, (payload) => handler(payload as T));
}
