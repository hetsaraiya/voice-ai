ALTER TABLE public.call_contexts
    ADD COLUMN IF NOT EXISTS callee_number character varying(50) NOT NULL DEFAULT '';
