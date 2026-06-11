ALTER TABLE public.assistant_api_deployments DROP COLUMN IF EXISTS greeting_interruptible;
ALTER TABLE public.assistant_debugger_deployments DROP COLUMN IF EXISTS greeting_interruptible;
ALTER TABLE public.assistant_phone_deployments DROP COLUMN IF EXISTS greeting_interruptible;
ALTER TABLE public.assistant_web_plugin_deployments DROP COLUMN IF EXISTS greeting_interruptible;
ALTER TABLE public.assistant_whatsapp_deployments DROP COLUMN IF EXISTS greeting_interruptible;
