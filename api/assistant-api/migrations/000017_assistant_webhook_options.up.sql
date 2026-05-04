CREATE TABLE public.assistant_webhook_options (
    id bigint PRIMARY KEY,
    status character varying(50) DEFAULT 'ACTIVE'::character varying NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    created_date timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_date timestamp without time zone,
    key character varying(200) NOT NULL,
    value text NOT NULL,
    assistant_webhook_id bigint NOT NULL
);

ALTER TABLE ONLY public.assistant_webhook_options
    ADD CONSTRAINT uk_assistant_webhook_option UNIQUE (key, assistant_webhook_id);

CREATE INDEX idx_assistant_webhook_options_assistant_webhook_id
    ON public.assistant_webhook_options USING btree (assistant_webhook_id);
