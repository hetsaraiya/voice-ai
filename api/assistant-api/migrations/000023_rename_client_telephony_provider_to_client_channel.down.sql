UPDATE public.assistant_conversation_metadata
SET
    key = 'client.telephony_provider',
    updated_date = CURRENT_TIMESTAMP
WHERE key = 'client.channel';
