import React from 'react';
import { UserOption } from './user-options';
import { TextImage } from '@/app/components/text-image';
import { User } from '@rapidaai/react';
import { RoleIndicator } from '@/app/components/indicators/role';
import { toHumanReadableDate } from '@/utils/date';
import { TableRow, TableCell } from '@carbon/react';
import { CarbonIconIndicator } from '@/app/components/carbon/icon-indicator';

export function SingleUser(props: { user: User }) {
  return (
    <TableRow>
      <TableCell>{props.user.getId()}</TableCell>
      <TableCell>
        <div className="flex items-center gap-3">
          <TextImage size={7} name={props.user.getName()} />
          <span>{props.user.getName()}</span>
        </div>
      </TableCell>
      <TableCell>{props.user.getEmail()}</TableCell>
      <TableCell>
        <RoleIndicator role={'SUPER_ADMIN'} />
      </TableCell>
      <TableCell>
        {props.user.getCreateddate() &&
          toHumanReadableDate(props.user.getCreateddate()!)}
      </TableCell>
      <TableCell>
        <CarbonIconIndicator state={props.user.getStatus?.()} />
      </TableCell>
      <TableCell>
        <UserOption id={props.user.getId()} />
      </TableCell>
    </TableRow>
  );
}
