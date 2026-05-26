import type { ElementType } from 'react';
import { Tag, Tooltip } from '@carbon/react';
import {
  Api,
  Application,
  Chat,
  Debug,
  Globe,
  PhoneVoice,
  Sdk,
  Unknown,
} from '@carbon/icons-react';
import type { ChannelIconName } from './channel';
import { normalizeChannel } from './channel';

const CHANNEL_ICON_SIZE = 16;

const CHANNEL_ICON_BY_NAME: Record<ChannelIconName, ElementType> = {
  api: Api,
  app: Application,
  chat: Chat,
  debugger: Debug,
  phone: PhoneVoice,
  sdk: Sdk,
  unknown: Unknown,
  web: Globe,
};

export const ChannelIndicator = ({ channel }: { channel: string }) => {
  const display = normalizeChannel(channel);
  const Icon = CHANNEL_ICON_BY_NAME[display.icon];

  return (
    <Tooltip align="bottom" label={display.tooltip}>
      <Tag size="md" type="gray" className="whitespace-nowrap">
        <span className="flex items-center gap-1.5 leading-none [&>svg]:block">
          <Icon size={CHANNEL_ICON_SIZE} />
          {display.label}
        </span>
      </Tag>
    </Tooltip>
  );
};
