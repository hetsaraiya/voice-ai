ALTER TABLE public.user_project_roles
    DROP CONSTRAINT IF EXISTS user_project_roles_user_project_status_key;

ALTER TABLE public.user_organization_roles
    DROP CONSTRAINT IF EXISTS user_organization_roles_user_org_status_key;
