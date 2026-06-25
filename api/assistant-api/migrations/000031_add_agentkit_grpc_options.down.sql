ALTER TABLE public.assistant_provider_agentkits
    DROP COLUMN IF EXISTS max_send_message_bytes,
    DROP COLUMN IF EXISTS max_recv_message_bytes,
    DROP COLUMN IF EXISTS keepalive_timeout_ms,
    DROP COLUMN IF EXISTS keepalive_time_ms,
    DROP COLUMN IF EXISTS connect_timeout_ms,
    DROP COLUMN IF EXISTS tls_server_name,
    DROP COLUMN IF EXISTS tls_verification,
    DROP COLUMN IF EXISTS transport_security;
