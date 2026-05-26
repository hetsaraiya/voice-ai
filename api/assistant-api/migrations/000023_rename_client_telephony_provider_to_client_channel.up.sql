UPDATE public.assistant_conversation_metadata
SET
    key = 'client.channel',
    updated_date = CURRENT_TIMESTAMP
WHERE key = 'client.telephony_provider';
