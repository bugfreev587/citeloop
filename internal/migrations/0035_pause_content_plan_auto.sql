-- Pause Content Plan automation for existing projects. Auto can be re-enabled
-- explicitly from the Content Plan switch, which requeues pending plan work.
update projects
set config = jsonb_set(config, '{auto_advance_enabled}', 'false'::jsonb, true)
where config->>'auto_advance_enabled' is distinct from 'false';
