import type { ReactNode } from 'react';

interface PageHeaderProps {
  kicker?: string;
  title: ReactNode;
  description?: string;
  actions?: ReactNode;
}

export function PageHeader({ kicker, title, description, actions }: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header-copy">
        {kicker && <div className="eyebrow"><span className="eyebrow-dash" />{kicker}</div>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="page-header-actions">{actions}</div>}
    </header>
  );
}
