ALTER TABLE public.assistant_webhooks
    ADD COLUMN IF NOT EXISTS project_id bigint,
    ADD COLUMN IF NOT EXISTS organization_id bigint;

UPDATE public.assistant_webhooks aw
SET
    project_id = a.project_id,
    organization_id = a.organization_id
FROM public.assistants a
WHERE aw.assistant_id = a.id
  AND (aw.project_id IS NULL OR aw.organization_id IS NULL);

ALTER TABLE public.assistant_webhooks
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN organization_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_assistant_webhooks_project_id
    ON public.assistant_webhooks USING btree (project_id);

CREATE INDEX IF NOT EXISTS idx_assistant_webhooks_organization_id
    ON public.assistant_webhooks USING btree (organization_id);

CREATE INDEX IF NOT EXISTS idx_assistant_webhooks_assistant_org_project
    ON public.assistant_webhooks USING btree (assistant_id, organization_id, project_id);


ALTER TABLE public.assistant_analyses
    ADD COLUMN IF NOT EXISTS project_id bigint,
    ADD COLUMN IF NOT EXISTS organization_id bigint;

UPDATE public.assistant_analyses aa
SET
    project_id = a.project_id,
    organization_id = a.organization_id
FROM public.assistants a
WHERE aa.assistant_id = a.id
  AND (aa.project_id IS NULL OR aa.organization_id IS NULL);

ALTER TABLE public.assistant_analyses
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN organization_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_assistant_analyses_project_id
    ON public.assistant_analyses USING btree (project_id);

CREATE INDEX IF NOT EXISTS idx_assistant_analyses_organization_id
    ON public.assistant_analyses USING btree (organization_id);

CREATE INDEX IF NOT EXISTS idx_assistant_analyses_assistant_org_project
    ON public.assistant_analyses USING btree (assistant_id, organization_id, project_id);


ALTER TABLE public.assistant_authentications
    ADD COLUMN IF NOT EXISTS project_id bigint,
    ADD COLUMN IF NOT EXISTS organization_id bigint;

UPDATE public.assistant_authentications aa
SET
    project_id = a.project_id,
    organization_id = a.organization_id
FROM public.assistants a
WHERE aa.assistant_id = a.id
  AND (aa.project_id IS NULL OR aa.organization_id IS NULL);

ALTER TABLE public.assistant_authentications
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN organization_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_assistant_authentications_project_id
    ON public.assistant_authentications USING btree (project_id);

CREATE INDEX IF NOT EXISTS idx_assistant_authentications_organization_id
    ON public.assistant_authentications USING btree (organization_id);

CREATE INDEX IF NOT EXISTS idx_assistant_authentications_assistant_org_project
    ON public.assistant_authentications USING btree (assistant_id, organization_id, project_id);
