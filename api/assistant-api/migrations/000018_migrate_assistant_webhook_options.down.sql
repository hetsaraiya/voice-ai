ALTER TABLE public.assistant_webhooks
    ADD COLUMN IF NOT EXISTS http_method text,
    ADD COLUMN IF NOT EXISTS http_url text,
    ADD COLUMN IF NOT EXISTS http_headers text DEFAULT '{}'::text,
    ADD COLUMN IF NOT EXISTS http_body text DEFAULT '{}'::text,
    ADD COLUMN IF NOT EXISTS retry_status_codes text NOT NULL DEFAULT '[]'::text,
    ADD COLUMN IF NOT EXISTS max_retry_count integer,
    ADD COLUMN IF NOT EXISTS timeout_seconds integer;

WITH http_method AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'http_method'
),
http_url AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'http_url'
),
http_headers AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'http_headers'
),
http_body AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'http_body'
),
retry_status_codes AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'retry_status_codes'
),
max_retry_count AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'max_retry_count'
),
timeout_seconds AS (
    SELECT assistant_webhook_id, value
    FROM public.assistant_webhook_options
    WHERE key = 'timeout_seconds'
)
UPDATE public.assistant_webhooks aw
SET
    http_method = COALESCE((SELECT hm.value FROM http_method hm WHERE hm.assistant_webhook_id = aw.id LIMIT 1), 'POST'::text),
    http_url = COALESCE((SELECT hu.value FROM http_url hu WHERE hu.assistant_webhook_id = aw.id LIMIT 1), ''::text),
    http_headers = COALESCE((SELECT hh.value FROM http_headers hh WHERE hh.assistant_webhook_id = aw.id LIMIT 1), '{}'::text),
    http_body = COALESCE((SELECT hb.value FROM http_body hb WHERE hb.assistant_webhook_id = aw.id LIMIT 1), '{}'::text),
    retry_status_codes = COALESCE((SELECT rs.value FROM retry_status_codes rs WHERE rs.assistant_webhook_id = aw.id LIMIT 1), '[]'::text),
    max_retry_count = COALESCE((SELECT CAST(NULLIF(mrc.value, '') AS integer) FROM max_retry_count mrc WHERE mrc.assistant_webhook_id = aw.id LIMIT 1), 0),
    timeout_seconds = COALESCE((SELECT CAST(NULLIF(ts.value, '') AS integer) FROM timeout_seconds ts WHERE ts.assistant_webhook_id = aw.id LIMIT 1), 0);
