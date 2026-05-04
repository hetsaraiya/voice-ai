CREATE SEQUENCE IF NOT EXISTS public.assistant_webhook_options_backfill_id_seq;

WITH seed AS (
    SELECT MAX(id) AS max_id
    FROM public.assistant_webhook_options
)
SELECT setval(
    'public.assistant_webhook_options_backfill_id_seq',
    COALESCE((SELECT max_id FROM seed), 1),
    COALESCE((SELECT max_id FROM seed), 0) > 0
);

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'http_method',
    COALESCE(aw.http_method, 'POST'::text),
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'http_url',
    COALESCE(aw.http_url, ''::text),
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'http_headers',
    COALESCE(aw.http_headers, '{}'::text),
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'http_body',
    COALESCE(aw.http_body, '{}'::text),
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'retry_status_codes',
    COALESCE(aw.retry_status_codes, '[]'::text),
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'max_retry_count',
    COALESCE(aw.max_retry_count, 0)::text,
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

INSERT INTO public.assistant_webhook_options (
    id,
    status,
    created_by,
    updated_by,
    created_date,
    updated_date,
    key,
    value,
    assistant_webhook_id
)
SELECT
    nextval('public.assistant_webhook_options_backfill_id_seq'),
    aw.status,
    aw.created_by,
    aw.updated_by,
    aw.created_date,
    aw.updated_date,
    'timeout_seconds',
    COALESCE(aw.timeout_seconds, 0)::text,
    aw.id
FROM public.assistant_webhooks aw
ON CONFLICT ON CONSTRAINT uk_assistant_webhook_option DO NOTHING;

DROP SEQUENCE IF EXISTS public.assistant_webhook_options_backfill_id_seq;

ALTER TABLE public.assistant_webhooks
    DROP COLUMN IF EXISTS http_method,
    DROP COLUMN IF EXISTS http_url,
    DROP COLUMN IF EXISTS http_headers,
    DROP COLUMN IF EXISTS http_body,
    DROP COLUMN IF EXISTS retry_status_codes,
    DROP COLUMN IF EXISTS max_retry_count,
    DROP COLUMN IF EXISTS timeout_seconds;
