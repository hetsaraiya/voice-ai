import React, { FC, useMemo } from 'react';
import {
  Checkbox,
  StructuredListBody,
  StructuredListCell,
  StructuredListHead,
  StructuredListRow,
  StructuredListWrapper,
} from '@carbon/react';
import { StorageFileGroup, storageFiles } from './storage-files';

export const StorageFileSelector: FC<{
  group: StorageFileGroup;
  selectedFiles: string[];
  onChange: (files: string[]) => void;
}> = ({ group, selectedFiles, onChange }) => {
  const selectedSet = useMemo(() => new Set(selectedFiles), [selectedFiles]);
  const groupFiles = storageFiles.filter(file => file.group === group);

  const updateFile = (fileId: string, checked: boolean) => {
    if (checked) {
      onChange(Array.from(new Set([...selectedFiles, fileId])));
      return;
    }
    onChange(selectedFiles.filter(selected => selected !== fileId));
  };

  return (
    <StructuredListWrapper
      aria-label={`${group} storage files`}
      isCondensed
      isFlush
    >
      <StructuredListHead>
        <StructuredListRow head>
          <StructuredListCell head>File</StructuredListCell>
          <StructuredListCell head>Description</StructuredListCell>
        </StructuredListRow>
      </StructuredListHead>
      <StructuredListBody>
        {groupFiles.map(file => (
          <StructuredListRow key={file.id}>
            <StructuredListCell noWrap>
              <Checkbox
                id={`storage-file-${file.id}`}
                checked={selectedSet.has(file.id)}
                onChange={(_, { checked }) =>
                  updateFile(file.id, Boolean(checked))
                }
                labelText={
                  <span className="font-mono text-[13px]">{file.name}</span>
                }
              />
            </StructuredListCell>
            <StructuredListCell>{file.description}</StructuredListCell>
          </StructuredListRow>
        ))}
      </StructuredListBody>
    </StructuredListWrapper>
  );
};
