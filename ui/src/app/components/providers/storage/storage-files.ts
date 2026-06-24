import { Metadata } from '@rapidaai/react';

export const STORAGE_FILES_OPTION_KEY = 'files_to_push';

export type StorageFileGroup = 'Recording';

export type StorageFileOption = {
  id: string;
  name: string;
  description: string;
  group: StorageFileGroup;
};

export const storageFiles: StorageFileOption[] = [
  {
    id: 'recording.conversation',
    name: 'recording.conversation',
    description: 'Mixed conversation audio with user and assistant channels.',
    group: 'Recording',
  },
  {
    id: 'recording.user',
    name: 'recording.user',
    description: 'User-side audio captured during the conversation.',
    group: 'Recording',
  },
  {
    id: 'recording.assistant',
    name: 'recording.assistant',
    description: 'Assistant-side generated audio from the conversation.',
    group: 'Recording',
  },
];

export const defaultStorageFiles = storageFiles.map(file => file.id);

export const parseSelectedStorageFiles = (parameters: Metadata[]): string[] => {
  const raw = parameters
    .find(param => param.getKey() === STORAGE_FILES_OPTION_KEY)
    ?.getValue();
  if (!raw) return defaultStorageFiles;

  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return defaultStorageFiles;
    const allowed = new Set(defaultStorageFiles);
    const selected = parsed.filter(
      (item): item is string => typeof item === 'string' && allowed.has(item),
    );
    return selected.length > 0 ? selected : defaultStorageFiles;
  } catch {
    return defaultStorageFiles;
  }
};

export const upsertStorageFilesOption = (
  parameters: Metadata[],
  selectedFiles: string[],
): Metadata[] => {
  const option = new Metadata();
  option.setKey(STORAGE_FILES_OPTION_KEY);
  option.setValue(JSON.stringify(selectedFiles));
  return [
    ...parameters.filter(param => param.getKey() !== STORAGE_FILES_OPTION_KEY),
    option,
  ];
};

export const preserveStorageConfigurationOptions = (
  parameters: Metadata[],
): Metadata[] =>
  parameters.filter(param =>
    ['rapida.credential_id', STORAGE_FILES_OPTION_KEY].includes(param.getKey()),
  );
