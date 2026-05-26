import { Tag, Tooltip } from '@carbon/react';
import { normalizeDisconnectReason } from './disconnect-reason';

export const DisconnectReasonIndicator = ({ reason }: { reason: string }) => {
  const display = normalizeDisconnectReason(reason);

  return (
    <Tooltip align="bottom" label={display.tooltip}>
      <Tag size="md" type="gray" className="inline-flex whitespace-nowrap">
        {display.label}
      </Tag>
    </Tooltip>
  );
};
