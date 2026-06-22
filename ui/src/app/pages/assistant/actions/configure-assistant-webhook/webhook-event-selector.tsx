import React, { FC, useMemo } from 'react';
import {
  Checkbox,
  StructuredListBody,
  StructuredListCell,
  StructuredListHead,
  StructuredListRow,
  StructuredListWrapper,
} from '@carbon/react';
import { WebhookEventGroup, webhookEvents } from './webhook-events';

export const WebhookEventSelector: FC<{
  group: WebhookEventGroup;
  selectedEvents: string[];
  onChange: (events: string[]) => void;
}> = ({ group, selectedEvents, onChange }) => {
  const selectedSet = useMemo(() => new Set(selectedEvents), [selectedEvents]);
  const groupEvents = webhookEvents.filter(event => event.group === group);

  const updateEvent = (eventId: string, checked: boolean) => {
    if (checked) {
      onChange(Array.from(new Set([...selectedEvents, eventId])));
      return;
    }
    onChange(selectedEvents.filter(selected => selected !== eventId));
  };

  return (
    <StructuredListWrapper
      aria-label={`${group} webhook events`}
      isCondensed
      isFlush
    >
      <StructuredListHead>
        <StructuredListRow head>
          <StructuredListCell head>Event</StructuredListCell>
          <StructuredListCell head>Description</StructuredListCell>
        </StructuredListRow>
      </StructuredListHead>
      <StructuredListBody>
        {groupEvents.map(event => (
          <StructuredListRow key={event.id}>
            <StructuredListCell noWrap>
              <Checkbox
                id={`webhook-event-${event.id}`}
                checked={selectedSet.has(event.id)}
                onChange={(_, { checked }) =>
                  updateEvent(event.id, Boolean(checked))
                }
                labelText={
                  <span className="font-mono text-[13px]">{event.name}</span>
                }
              />
            </StructuredListCell>
            <StructuredListCell>{event.description}</StructuredListCell>
          </StructuredListRow>
        ))}
      </StructuredListBody>
    </StructuredListWrapper>
  );
};
