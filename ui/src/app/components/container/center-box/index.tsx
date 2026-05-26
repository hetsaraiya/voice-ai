import { FC } from 'react';

/**
 * Flex box usually a parent container for all the page objects
 */
interface CenterBoxProps extends React.HTMLAttributes<HTMLElement> {}

export const CenterBox: FC<CenterBoxProps> = props => {
  return (
    <div className="grid min-h-dvh grid-cols-[1fr_2.5rem_minmax(0,var(--container-2xl))_2.5rem_1fr] grid-rows-[1fr_auto_1fr] overflow-clip">
      <div className="col-start-2 row-span-full row-start-1 border-x border-gray-200 dark:border-gray-900" />
      <div className="col-start-4 row-span-full border-x border-gray-200 dark:border-gray-900" />
      <main className="grid grid-cols-1 col-start-3 row-start-2 border-y border-gray-200 dark:border-gray-900">
        <div className="grid! grid-cols-1! items-center! bg-white p-10! dark:bg-gray-950">
          <div className="grid grid-cols-1 gap-10 w-full">{props.children}</div>
        </div>
      </main>
    </div>
  );
};
