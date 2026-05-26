import type { ElementType, FC, ReactNode } from 'react';
import { Button } from '@carbon/react';
import { cn } from '@/utils';

export interface EmptyStateProps {
  icon?: ElementType;
  title: string;
  subtitle?: string;
  action?: string;
  actionIcon?: ElementType;
  onAction?: () => void;
  actionComponent?: ReactNode;
  className?: string;
}

export const EmptyState: FC<EmptyStateProps> = ({
  icon: Icon,
  title,
  subtitle,
  action,
  actionIcon,
  onAction,
  actionComponent,
  className,
}) => {
  return (
    <div
      className={cn(
        'flex flex-1 flex-col items-center justify-center px-8',
        className,
      )}
    >
      {Icon && (
        <div className="w-16 h-16 flex items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800 mb-5">
          <Icon size={32} className="text-gray-400 dark:text-gray-500" />
        </div>
      )}
      <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-2">
        {title}
      </h3>
      {subtitle && (
        <p className="text-sm text-gray-500 dark:text-gray-400 text-center max-w-md mb-5">
          {subtitle}
        </p>
      )}
      {actionComponent}
      {action && onAction && (
        <Button
          size="lg"
          kind="tertiary"
          renderIcon={actionIcon}
          className="dark:bg-gray-950! bg-white! dark:hover:text-white! hover:bg-primary! dark:hover:bg-primary! font-medium"
          onClick={onAction}
        >
          {action}
        </Button>
      )}
    </div>
  );
};
