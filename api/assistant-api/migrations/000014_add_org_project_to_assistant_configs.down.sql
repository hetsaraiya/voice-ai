DROP INDEX IF EXISTS public.idx_assistant_authentications_assistant_org_project;
DROP INDEX IF EXISTS public.idx_assistant_authentications_organization_id;
DROP INDEX IF EXISTS public.idx_assistant_authentications_project_id;

ALTER TABLE public.assistant_authentications
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS project_id;


DROP INDEX IF EXISTS public.idx_assistant_analyses_assistant_org_project;
DROP INDEX IF EXISTS public.idx_assistant_analyses_organization_id;
DROP INDEX IF EXISTS public.idx_assistant_analyses_project_id;

ALTER TABLE public.assistant_analyses
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS project_id;


DROP INDEX IF EXISTS public.idx_assistant_webhooks_assistant_org_project;
DROP INDEX IF EXISTS public.idx_assistant_webhooks_organization_id;
DROP INDEX IF EXISTS public.idx_assistant_webhooks_project_id;

ALTER TABLE public.assistant_webhooks
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS project_id;
