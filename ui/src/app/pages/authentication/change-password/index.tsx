import React, { useCallback, useState } from 'react';
import { Helmet } from '@/app/components/helmet';
import { useNavigate, useParams } from 'react-router-dom';
import { CreatePassword } from '@rapidaai/react';
import { CreatePasswordResponse } from '@rapidaai/react';
import { useForm } from 'react-hook-form';
import { ServiceError } from '@rapidaai/react';
import { connectionConfig } from '@/configs';
import { useRapidaStore } from '@/hooks';
import { Stack } from '@/app/components/carbon/form';
import { PrimaryButton } from '@/app/components/carbon/button';
import { Notification } from '@/app/components/carbon/notification';
import { ArrowRight } from '@carbon/icons-react';
import { PasswordInput } from '@carbon/react';

export function ChangePasswordPage() {
  const { register, handleSubmit } = useForm();
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const { token } = useParams();
  const { loading, showLoader, hideLoader } = useRapidaStore();

  const afterCreatePassword = useCallback(
    (err: ServiceError | null, cpr: CreatePasswordResponse | null) => {
      hideLoader();
      if (err) {
        setError('Unable to process your request. Please try again later.');
        return;
      }
      if (cpr?.getSuccess()) {
        return navigate('/auth/signin');
      } else {
        let errorMessage = cpr?.getError();
        if (errorMessage) setError(errorMessage.getHumanmessage());
        else
          setError('Unable to process your request. Please try again later.');
        return;
      }
    },
    [],
  );

  const onCreatePassword = data => {
    if (!token) {
      setError(
        'The password token is expired, please request again for reset password token.',
      );
      return;
    }
    if (data.password !== data.confirmPassword) {
      setError('Passwords entered do not match, please check and try again.');
      return;
    }
    showLoader();
    CreatePassword(connectionConfig, token, data.password, afterCreatePassword);
  };

  return (
    <Stack gap={6}>
      <Helmet title="Change your password" />
      <Stack gap={2}>
        <h1 className="m-0 text-[1.8rem] leading-tight">Change Password</h1>
        <p className="mt-1.5 text-sm leading-[1.4286] text-(--cds-text-secondary)">
          You've requested to change your password. Please enter your new
          password below to secure your account.
        </p>
        <div
          aria-hidden="true"
          className={`h-px bg-gray-300 dark:bg-gray-900 mt-3`}
        />
      </Stack>
      <form className="mt-6" onSubmit={handleSubmit(onCreatePassword)}>
        <Stack gap={5}>
          <PasswordInput
            id="new-password"
            labelText="Password"
            required
            placeholder="********"
            {...register('password')}
          />
          <PasswordInput
            id="confirm-password"
            labelText="Confirm Password"
            required
            placeholder="********"
            {...register('confirmPassword')}
          />
          {error && (
            <Notification kind="error" title="Error" subtitle={error} />
          )}
          <PrimaryButton
            size="lg"
            renderIcon={ArrowRight}
            type="submit"
            isLoading={loading}
            className="!w-full !max-w-none !justify-between"
          >
            Change Password
          </PrimaryButton>
        </Stack>
      </form>
    </Stack>
  );
}
