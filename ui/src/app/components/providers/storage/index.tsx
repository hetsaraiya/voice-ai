import { Metadata, VaultCredential } from '@rapidaai/react';
import { CredentialDropdown } from '@/app/components/dropdown/credential-dropdown';
import { useCallback } from 'react';
import { ProviderComponentProps } from '@/app/components/providers';
import { STORAGE_PROVIDER } from '@/providers';
import { Dropdown } from '@carbon/react';
import { Stack } from '@/app/components/carbon/form';
import { loadProviderConfig } from '@/providers/config-loader';
import {
  getDefaultsFromConfig,
  validateFromConfig,
} from '@/providers/config-defaults';
import { ConfigRenderer } from '@/app/components/providers/config-renderer';
import {
  preserveStorageConfigurationOptions,
  STORAGE_FILES_OPTION_KEY,
} from './storage-files';
export {
  defaultStorageFiles,
  parseSelectedStorageFiles,
  preserveStorageConfigurationOptions,
  STORAGE_FILES_OPTION_KEY,
  storageFiles,
  upsertStorageFilesOption,
} from './storage-files';
export { StorageFileSelector } from './storage-file-selector';

export const GetDefaultStorageConfigIfInvalid = (
  provider: string,
  parameters: Metadata[],
): Metadata[] => {
  const config = loadProviderConfig(provider);
  if (!config?.storage) return [];
  const normalized = getDefaultsFromConfig(
    config,
    'storage',
    parameters,
    provider,
    { includeCredential: false },
  );
  const preserved = preserveStorageConfigurationOptions(parameters);
  if (!preserved.some(param => param.getKey() === 'rapida.credential_id')) {
    const credential = new Metadata();
    credential.setKey('rapida.credential_id');
    credential.setValue('');
    preserved.unshift(credential);
  }
  if (!preserved.some(param => param.getKey() === STORAGE_FILES_OPTION_KEY)) {
    const filesToPush = new Metadata();
    filesToPush.setKey(STORAGE_FILES_OPTION_KEY);
    filesToPush.setValue('');
    preserved.push(filesToPush);
  }
  return [...preserved, ...normalized];
};

export const ValidateStorageOptions = (
  provider: string,
  parameters: Metadata[],
): boolean => {
  const config = loadProviderConfig(provider);
  if (!config?.storage) return false;
  return !validateFromConfig(config, 'storage', provider, parameters);
};

export const ConfigureStorageComponent: React.FC<ProviderComponentProps> = ({
  provider,
  parameters,
  onChangeParameter,
}) => {
  const config = loadProviderConfig(provider);
  if (!config?.storage) return null;

  return (
    <ConfigRenderer
      provider={provider}
      category="storage"
      config={config.storage}
      parameters={parameters}
      onParameterChange={onChangeParameter}
    />
  );
};

/**
 *
 * @param param0
 * @returns
 */
export const CloudStorageProvider: React.FC<ProviderComponentProps> = ({
  parameters,
  provider,
  onChangeParameter,
  onChangeProvider,
}) => {
  const getParamValue = useCallback(
    (key: string) => {
      return parameters?.find(p => p.getKey() === key)?.getValue() ?? '';
    },
    [JSON.stringify(parameters)],
  );

  const updateParameter = (key: string, value: string) => {
    const updatedParams = [...(parameters || [])];
    const existingIndex = updatedParams.findIndex(p => p.getKey() === key);
    const newParam = new Metadata();
    newParam.setKey(key);
    newParam.setValue(value);
    if (existingIndex >= 0) {
      updatedParams[existingIndex] = newParam;
    } else {
      updatedParams.push(newParam);
    }
    onChangeParameter(updatedParams);
  };

  const selectedProvider =
    STORAGE_PROVIDER.find(x => x.code === provider) || null;

  return (
    <Stack gap={6}>
      <Dropdown
        id="storage-provider"
        titleText="Storage provider"
        label="Select storage provider"
        items={STORAGE_PROVIDER}
        selectedItem={selectedProvider}
        itemToString={(item: any) => item?.name || ''}
        onChange={({ selectedItem }: any) => {
          if (!selectedItem) return;
          onChangeProvider(selectedItem.code);
          onChangeParameter(
            GetDefaultStorageConfigIfInvalid(
              selectedItem.code,
              parameters || [],
            ),
          );
        }}
        helperText="Select a storage provider for assistant recordings."
      />
      {provider && (
        <CredentialDropdown
          onChangeCredential={(c: VaultCredential) => {
            updateParameter('rapida.credential_id', c.getId());
          }}
          currentCredential={getParamValue('rapida.credential_id')}
          provider={provider}
        />
      )}
      {provider && (
        <div className="grid grid-cols-3 gap-x-6 gap-y-3">
          <ConfigureStorageComponent
            parameters={parameters || []}
            provider={provider}
            onChangeParameter={onChangeParameter}
            onChangeProvider={onChangeProvider}
          />
        </div>
      )}
    </Stack>
  );
};
