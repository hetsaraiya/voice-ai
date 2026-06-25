import { FC } from 'react';
import { TextInput } from '@/app/components/carbon/form';
import { TertiaryButton } from '@/app/components/carbon/button';
import { Add, TrashCan } from '@carbon/icons-react';
import { Button } from '@carbon/react';

interface Parameter {
  key: string;
  value: string;
}

export const APiParameter: FC<{
  inputClass?: string;
  initialValues: Parameter[];
  setParameterValue: (params: Parameter[]) => void;
  actionButtonLabel?: string;
}> = ({
  initialValues,
  setParameterValue,
  actionButtonLabel = 'Add new pair',
}) => {
  const emptyLabel = actionButtonLabel.replace(/^Add\s+/i, '').toLowerCase();

  const updateParameter = (
    index: number,
    field: 'key' | 'value',
    value: string,
  ) => {
    const updatedParameters = [...initialValues];
    updatedParameters[index][field] = value;
    setParameterValue(updatedParameters);
  };

  return (
    <>
      <table className="w-full border-collapse border border-gray-200 dark:border-gray-700 text-sm [&_input]:!border-none [&_.cds--text-input]:!border-none [&_.cds--text-input]:!outline-none [&_.cds--form-item]:!m-0">
        <thead>
          <tr className="bg-gray-50 dark:bg-gray-900">
            <th className="text-left text-xs font-medium text-gray-500 dark:text-gray-400 px-3 py-2 border-b border-r border-gray-200 dark:border-gray-700 w-1/2">
              Key
            </th>
            <th className="text-left text-xs font-medium text-gray-500 dark:text-gray-400 px-3 py-2 border-b border-r border-gray-200 dark:border-gray-700 w-1/2">
              Value
            </th>
            <th className="border-b border-gray-200 dark:border-gray-700 w-8" />
          </tr>
        </thead>
        <tbody>
          {initialValues.length === 0 && (
            <tr>
              <td
                colSpan={3}
                className="px-4 py-3 text-xs text-gray-500 dark:text-gray-400"
              >
                No {emptyLabel} yet. Click <strong>{actionButtonLabel}</strong>{' '}
                below.
              </td>
            </tr>
          )}
          {initialValues.map((parameter, index) => (
            <tr
              key={index}
              className="border-b border-gray-200 dark:border-gray-700 last:border-b-0"
            >
              <td className="border-r border-gray-200 dark:border-gray-700 p-0">
                <TextInput
                  id={`api-param-key-${index}`}
                  labelText=""
                  hideLabel
                  value={parameter.key}
                  onChange={e => updateParameter(index, 'key', e.target.value)}
                  placeholder="Key"
                  size="md"
                />
              </td>
              <td className="border-r border-gray-200 dark:border-gray-700 p-0">
                <TextInput
                  id={`api-param-val-${index}`}
                  labelText=""
                  hideLabel
                  value={parameter.value}
                  onChange={e =>
                    updateParameter(index, 'value', e.target.value)
                  }
                  placeholder="Value"
                  size="md"
                />
              </td>
              <td className="p-0 text-center">
                <Button
                  hasIconOnly
                  renderIcon={TrashCan}
                  iconDescription="Remove"
                  kind="danger--ghost"
                  size="sm"
                  onClick={() =>
                    setParameterValue(
                      initialValues.filter((_, i) => i !== index),
                    )
                  }
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="pt-2">
        <TertiaryButton
          size="md"
          renderIcon={Add}
          onClick={() =>
            setParameterValue([...initialValues, { key: '', value: '' }])
          }
          className="!w-full !max-w-none"
        >
          {actionButtonLabel}
        </TertiaryButton>
      </div>
    </>
  );
};
