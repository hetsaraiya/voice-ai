ALTER TABLE public.assistant_provider_agentkits
    ADD COLUMN transport_security character varying(50) DEFAULT 'TLS' NOT NULL,
    ADD COLUMN tls_verification character varying(50) DEFAULT 'VERIFY' NOT NULL,
    ADD COLUMN tls_server_name character varying(255),
    ADD COLUMN connect_timeout_ms bigint DEFAULT 10000 NOT NULL,
    ADD COLUMN keepalive_time_ms bigint DEFAULT 30000 NOT NULL,
    ADD COLUMN keepalive_timeout_ms bigint DEFAULT 10000 NOT NULL,
    ADD COLUMN max_recv_message_bytes bigint DEFAULT 16777216 NOT NULL,
    ADD COLUMN max_send_message_bytes bigint DEFAULT 16777216 NOT NULL;
