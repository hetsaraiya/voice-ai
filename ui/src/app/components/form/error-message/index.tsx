import { RedNoticeBlock } from '@/app/components/container/message/notice-block';
import React, { FC, HTMLAttributes } from 'react';
import { Notification } from '@/app/components/carbon/notification';

interface ErrorMessageProps extends HTMLAttributes<HTMLDivElement> {
  message?: string;
}

export const ErrorMessage: FC<ErrorMessageProps> = (
  props: ErrorMessageProps,
) => {
  if (!props.message) return <></>;
  return <Notification kind="error" title="Error" subtitle={props.message} />
};
