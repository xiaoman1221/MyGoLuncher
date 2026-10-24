import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon: ReactNode;
  title: string;
  body: string;
  action?: ReactNode;
  compact?: boolean;
}

export function EmptyState({ icon, title, body, action, compact }: EmptyStateProps) {
  return (
    <div className={`empty-state ${compact ? 'empty-state-compact' : ''}`}>
      <div className="empty-state-icon">{icon}</div>
      <h3>{title}</h3>
      <p>{body}</p>
      {action && <div className="empty-state-action">{action}</div>}
    </div>
  );
}
