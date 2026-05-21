DELETE FROM public.user_organization_roles
WHERE id IN (
    SELECT id
    FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY user_auth_id, organization_id, status
            ORDER BY id DESC
        ) AS row_number
        FROM public.user_organization_roles
    ) duplicate_roles
    WHERE row_number > 1
);

ALTER TABLE public.user_organization_roles
    ADD CONSTRAINT user_organization_roles_user_org_status_key UNIQUE (user_auth_id, organization_id, status);

DELETE FROM public.user_project_roles
WHERE id IN (
    SELECT id
    FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY user_auth_id, project_id, status
            ORDER BY id DESC
        ) AS row_number
        FROM public.user_project_roles
    ) duplicate_roles
    WHERE row_number > 1
);

ALTER TABLE public.user_project_roles
    ADD CONSTRAINT user_project_roles_user_project_status_key UNIQUE (user_auth_id, project_id, status);
