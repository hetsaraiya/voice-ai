import { FC } from 'react';
import RawIconIndicator from '@carbon/react/es/components/IconIndicator';

export type CarbonIconIndicatorKind =
  | 'failed'
  | 'caution-major'
  | 'caution-minor'
  | 'undefined'
  | 'succeeded'
  | 'normal'
  | 'in-progress'
  | 'incomplete'
  | 'not-started'
  | 'pending'
  | 'unknown'
  | 'informative';

export interface CarbonIconIndicatorProps {
  state?: string;
  kind?: CarbonIconIndicatorKind;
  label?: string;
  size?: 16 | 20 | 24 | 32;
}

export const CarbonIconIndicator: FC<CarbonIconIndicatorProps> = ({
  state,
  kind,
  label,
  size = 16,
}) => {
  const resolved = state
    ? recordStateToIconIndicator[state] || defaultRecordIconIndicator
    : {
        kind: kind || defaultRecordIconIndicator.kind,
        label: label || defaultRecordIconIndicator.label,
      };

  return (
    <RawIconIndicator
      kind={resolved.kind as any}
      label={resolved.label}
      size={size}
    />
  );
};

export const recordStateToIconIndicator: Record<
  string,
  { kind: CarbonIconIndicatorKind; label: string }
> = {
  RECORD_ACTIVE: { kind: 'normal', label: 'Active' },
  ACTIVE: { kind: 'normal', label: 'Active' },
  active: { kind: 'normal', label: 'Active' },

  RECORD_INVITED: { kind: 'incomplete', label: 'Invited' },
  INVITED: { kind: 'incomplete', label: 'Invited' },
  invited: { kind: 'incomplete', label: 'Invited' },

  RECORD_QUEUED: { kind: 'pending', label: 'Queued' },
  QUEUED: { kind: 'pending', label: 'Queued' },
  queued: { kind: 'pending', label: 'Queued' },

  RECORD_CONNECTED: { kind: 'succeeded', label: 'Connected' },
  CONNECTED: { kind: 'succeeded', label: 'Connected' },
  connected: { kind: 'succeeded', label: 'Connected' },

  RECORD_IN_PROGRESS: { kind: 'in-progress', label: 'In Progress' },
  IN_PROGRESS: { kind: 'in-progress', label: 'In Progress' },
  in_progress: { kind: 'in-progress', label: 'In Progress' },

  RECORD_SUCCESS: { kind: 'succeeded', label: 'Success' },
  SUCCESS: { kind: 'succeeded', label: 'Success' },
  success: { kind: 'succeeded', label: 'Success' },

  RECORD_COMPLETE: { kind: 'succeeded', label: 'Complete' },
  COMPLETE: { kind: 'succeeded', label: 'Complete' },
  complete: { kind: 'succeeded', label: 'Complete' },
  COMPLETED: { kind: 'succeeded', label: 'Completed' },
  completed: { kind: 'succeeded', label: 'Completed' },

  RECORD_INACTIVE: { kind: 'incomplete', label: 'Inactive' },
  INACTIVE: { kind: 'incomplete', label: 'Inactive' },
  inactive: { kind: 'incomplete', label: 'Inactive' },
  DISABLED: { kind: 'not-started', label: 'Disabled' },
  disabled: { kind: 'not-started', label: 'Disabled' },

  RECORD_ARCHIEVE: { kind: 'incomplete', label: 'Archived' },
  ARCHIEVE: { kind: 'incomplete', label: 'Archived' },
  archived: { kind: 'incomplete', label: 'Archived' },

  RECORD_FAILED: { kind: 'failed', label: 'Failed' },
  FAILED: { kind: 'failed', label: 'Failed' },
  failed: { kind: 'failed', label: 'Failed' },
};

export const defaultRecordIconIndicator = {
  kind: 'unknown' as CarbonIconIndicatorKind,
  label: 'Unknown',
};
