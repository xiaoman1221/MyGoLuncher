import { isDesktop } from './api';

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
  let disposed = false;
  let unsubscribe = () => {};
  void import('./mock').then(({ mockSubscribe }) => {
    if (!disposed) unsubscribe = mockSubscribe(event, (payload) => handler(payload as T));
  });
  return () => {
    disposed = true;
    unsubscribe();
  };
}
