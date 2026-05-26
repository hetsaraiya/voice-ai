import { Metadata } from '@rapidaai/react';
import { TextInput } from '@/app/components/carbon/form';

export const ValidateTelnyxTelephonyOptions = (
  options: Metadata[],
): boolean => {
  const credentialID = options.find(
    opt => opt.getKey() === 'rapida.credential_id',
  );
  if (!credentialID?.getValue()) return false;
  const phone = options.find(opt => opt.getKey() === 'phone');
  if (phone && !phone.getValue()) return false;
  return true;
};

export const ConfigureTelnyxTelephony: React.FC<{
  onParameterChange: (parameters: Metadata[]) => void;
  parameters: Metadata[] | null;
}> = ({ onParameterChange, parameters }) => {
  const getParamValue = (key: string) =>
    parameters?.find(p => p.getKey() === key)?.getValue() ?? '';

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
    onParameterChange(updatedParams);
  };

  return (
    <div className="col-span-2">
      <TextInput
        id="telnyx-phone"
        labelText="Phone"
        value={getParamValue('phone')}
        onChange={e => updateParameter('phone', e.target.value)}
        placeholder="Enter your Telnyx phone number"
        helperText="Phone to receive inbound or make outbound call."
      />
    </div>
  );
};
